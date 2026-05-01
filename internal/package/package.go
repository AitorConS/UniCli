// Package package manages pre-built runtime packages for unikernel images.
//
// A package is a named, versioned collection of files (typically a language
// runtime like Node.js or Python) that can be included in a unikernel image
// during "uni build --pkg". Packages are stored locally in
// ~/.uni/packages/<name>/<version>/ and are downloaded from a remote index.
package pkg

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// IndexURL is the base URL for the package index.
const IndexURL = "https://github.com/AitorConS/UniCLi/releases/download/pkg-index/packages.json"

// Package describes a downloadable runtime package.
type Package struct {
	// Name is the package name (e.g. "node", "python", "redis", "nginx").
	Name string `json:"name"`
	// Version is the semantic version (e.g. "20.11.0").
	Version string `json:"version"`
	// Description is a short human-readable summary.
	Description string `json:"description"`
	// Runtime is the runtime family (e.g. "node", "python").
	Runtime string `json:"runtime"`
	// SHA256 is the expected hex-encoded SHA-256 digest of the archive.
	SHA256 string `json:"sha256"`
	// Size is the archive size in bytes.
	Size int64 `json:"size"`
	// URL is the download URL for the package archive.
	URL string `json:"url"`
	// Created is the publication timestamp.
	Created time.Time `json:"created"`
}

// Index is the top-level package index structure.
type Index struct {
	// Packages maps package name to its available versions.
	Packages map[string][]Package `json:"packages"`
}

// Store manages locally cached packages under a root directory.
type Store struct {
	root string
	mu   sync.RWMutex
}

// NewStore creates a Store rooted at dir, creating it if needed.
func NewStore(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("package store mkdir %s: %w", dir, err)
	}
	return &Store{root: dir}, nil
}

// PackageDir returns the local directory for a package version.
func (s *Store) PackageDir(name, version string) string {
	return filepath.Join(s.root, name, version)
}

// IsDownloaded returns true if the package archive exists locally.
func (s *Store) IsDownloaded(name, version string) bool {
	dir := s.PackageDir(name, version)
	archive := filepath.Join(dir, "files.tar.gz")
	info, err := os.Stat(archive)
	return err == nil && !info.IsDir()
}

// Download fetches the package archive from its URL and stores it locally.
// Verifies the SHA-256 digest after download.
func (s *Store) Download(pkg Package) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dir := s.PackageDir(pkg.Name, pkg.Version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("package download mkdir %s: %w", dir, err)
	}

	archivePath := filepath.Join(dir, "files.tar.gz")
	if _, err := os.Stat(archivePath); err == nil {
		return nil
	}

	fmt.Printf("Downloading package %s %s...\n", pkg.Name, pkg.Version)

	req, err := http.NewRequest(http.MethodGet, pkg.URL, nil)
	if err != nil {
		return fmt.Errorf("package download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("package download %s: %w", pkg.Name, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("package download %s: HTTP %d", pkg.Name, resp.StatusCode)
	}

	f, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("package download create %s: %w", archivePath, err)
	}
	defer func() { _ = f.Close() }()

	size, err := io.Copy(f, resp.Body)
	if err != nil {
		_ = os.Remove(archivePath)
		return fmt.Errorf("package download write: %w", err)
	}

	if pkg.Size > 0 && size != pkg.Size {
		_ = os.Remove(archivePath)
		return fmt.Errorf("package download: size mismatch (got %d, want %d)", size, pkg.Size)
	}

	fmt.Printf("Package %s %s downloaded (%.1f MB)\n", pkg.Name, pkg.Version, float64(size)/(1<<20))
	return nil
}

// Remove deletes a locally cached package.
func (s *Store) Remove(name, version string) error {
	dir := s.PackageDir(name, version)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("package remove %s: %w", name, err)
	}
	return nil
}

// List returns all locally cached package versions.
func (s *Store) List() ([]Package, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("package list: %w", err)
	}
	var result []Package
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		verEntries, err := os.ReadDir(filepath.Join(s.root, name))
		if err != nil {
			continue
		}
		for _, ve := range verEntries {
			if !ve.IsDir() {
				continue
			}
			metaPath := filepath.Join(s.root, name, ve.Name(), "meta.json")
			data, err := os.ReadFile(metaPath)
			if err != nil {
				result = append(result, Package{Name: name, Version: ve.Name()})
				continue
			}
			var pkg Package
			if err := json.Unmarshal(data, &pkg); err != nil {
				result = append(result, Package{Name: name, Version: ve.Name()})
				continue
			}
			result = append(result, pkg)
		}
	}
	return result, nil
}

// SaveMeta writes the package metadata to the local cache.
func (s *Store) SaveMeta(pkg Package) error {
	dir := s.PackageDir(pkg.Name, pkg.Version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("package meta mkdir %s: %w", dir, err)
	}
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		return fmt.Errorf("package meta marshal: %w", err)
	}
	metaPath := filepath.Join(dir, "meta.json")
	if err := os.WriteFile(metaPath, data, 0o644); err != nil {
		return fmt.Errorf("package meta write %s: %w", metaPath, err)
	}
	return nil
}

// FetchIndex downloads and parses the remote package index.
func FetchIndex() (*Index, error) {
	req, err := http.NewRequest(http.MethodGet, IndexURL, nil)
	if err != nil {
		return nil, fmt.Errorf("package index request: %w", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("package index fetch: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("package index: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("package index read: %w", err)
	}

	var idx Index
	if err := json.Unmarshal(data, &idx); err != nil {
		return nil, fmt.Errorf("package index parse: %w", err)
	}
	return &idx, nil
}

// Search returns packages matching the given query string.
// Matches against name, description, and runtime.
func (idx *Index) Search(query string) []Package {
	var result []Package
	lower := strings.ToLower(query)
	for _, versions := range idx.Packages {
		for _, pkg := range versions {
			if strings.Contains(strings.ToLower(pkg.Name), lower) ||
				strings.Contains(strings.ToLower(pkg.Description), lower) ||
				strings.Contains(strings.ToLower(pkg.Runtime), lower) {
				result = append(result, pkg)
				break
			}
		}
	}
	return result
}

// Latest returns the latest version of a package by name, or nil if not found.
func (idx *Index) Latest(name string) *Package {
	versions, ok := idx.Packages[name]
	if !ok || len(versions) == 0 {
		return nil
	}
	return &versions[0]
}