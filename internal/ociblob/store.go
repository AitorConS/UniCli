package ociblob

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// Store stores OCI blobs in a content-addressable directory.
type Store struct {
	root string
	mu   sync.RWMutex
}

// NewStore creates a Store rooted at root.
func NewStore(root string) (*Store, error) {
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("blob store mkdir %s: %w", root, err)
	}
	return &Store{root: root}, nil
}

// Put stores data and returns its digest and size.
func (s *Store) Put(r io.Reader) (string, int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	tmp, err := os.CreateTemp(s.root, "blob-*")
	if err != nil {
		return "", 0, fmt.Errorf("blob store put create temp: %w", err)
	}
	defer func() {
		_ = tmp.Close()
		_ = os.Remove(tmp.Name())
	}()

	h := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, h), r)
	if err != nil {
		return "", 0, fmt.Errorf("blob store put write temp: %w", err)
	}
	digest := "sha256:" + hex.EncodeToString(h.Sum(nil))

	if err := tmp.Close(); err != nil {
		return "", 0, fmt.Errorf("blob store put close temp: %w", err)
	}

	dst, err := s.pathForDigest(digest)
	if err != nil {
		return "", 0, fmt.Errorf("blob store put: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", 0, fmt.Errorf("blob store put mkdir: %w", err)
	}

	if _, err := os.Stat(dst); err == nil {
		return digest, size, nil
	}
	if err := os.Rename(tmp.Name(), dst); err != nil {
		if _, statErr := os.Stat(dst); statErr == nil {
			return digest, size, nil
		}
		return "", 0, fmt.Errorf("blob store put move blob: %w", err)
	}

	return digest, size, nil
}

// Exists reports whether digest exists in the store.
func (s *Store) Exists(digest string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	path, err := s.pathForDigest(digest)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// Open opens digest for reading.
func (s *Store) Open(digest string) (io.ReadCloser, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path, err := s.pathForDigest(digest)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("blob %s not found", digest)
		}
		return nil, fmt.Errorf("open blob %s: %w", digest, err)
	}
	return f, nil
}

// Delete removes digest from the store.
func (s *Store) Delete(digest string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	path, err := s.pathForDigest(digest)
	if err != nil {
		return err
	}
	err = os.Remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("delete blob %s: %w", digest, err)
	}
	return nil
}

// List returns digests currently in the store.
func (s *Store) List() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	entries, err := os.ReadDir(s.root)
	if err != nil {
		return nil, fmt.Errorf("list blobs: %w", err)
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !isHexSHA256(name) {
			continue
		}
		out = append(out, "sha256:"+name)
	}
	sort.Strings(out)
	return out, nil
}

// pathForDigest maps a "sha256:<hex>" digest to its on-disk path. The digest is
// content-addressable and untrusted (it can come from a remote manifest), so the
// hex portion is validated to be exactly 64 hex characters before it is joined
// onto the store root — otherwise a value like "sha256:../../etc/passwd" would
// let a caller read, write, or delete outside the store.
func (s *Store) pathForDigest(digest string) (string, error) {
	hexDigest := strings.TrimPrefix(digest, "sha256:")
	if !isHexSHA256(hexDigest) {
		return "", fmt.Errorf("invalid blob digest %q", digest)
	}
	return filepath.Join(s.root, hexDigest), nil
}

func isHexSHA256(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, r := range s {
		if (r >= '0' && r <= '9') || (r >= 'a' && r <= 'f') {
			continue
		}
		return false
	}
	return true
}
