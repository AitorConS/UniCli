package pkg

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStore_CreateAndList(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	pkg := Package{
		Name:        "node",
		Version:     "20.11.0",
		Description: "Node.js JavaScript runtime",
		Runtime:     "node",
		SHA256:      "abc123",
		Size:        1024,
	}
	require.NoError(t, store.SaveMeta(pkg))

	pkgs, err := store.List()
	require.NoError(t, err)
	require.Len(t, pkgs, 1)
	require.Equal(t, "node", pkgs[0].Name)
	require.Equal(t, "20.11.0", pkgs[0].Version)
}

func TestStore_Remove(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	pkg := Package{Name: "python", Version: "3.12.0"}
	require.NoError(t, store.SaveMeta(pkg))

	require.NoError(t, store.Remove("python", "3.12.0"))

	pkgs, err := store.List()
	require.NoError(t, err)
	require.Len(t, pkgs, 0)
}

func TestStore_Remove_NotFound(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	require.NoError(t, store.Remove("nonexistent", "1.0.0"))
}

func TestStore_PackageDir(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	result := store.PackageDir("node", "20.11.0")
	require.Equal(t, filepath.Join(dir, "node", "20.11.0"), result)
}

func TestStore_IsDownloaded(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	require.False(t, store.IsDownloaded("node", "20.11.0"))

	pkgDir := store.PackageDir("node", "20.11.0")
	require.NoError(t, os.MkdirAll(pkgDir, 0o755))
	archivePath := filepath.Join(pkgDir, "files.tar.gz")
	require.NoError(t, os.WriteFile(archivePath, []byte("fake"), 0o644))

	require.True(t, store.IsDownloaded("node", "20.11.0"))
}

func TestIndex_Search(t *testing.T) {
	idx := &Index{
		Packages: map[string][]Package{
			"node":   {{Name: "node", Version: "20.11.0", Description: "Node.js runtime", Runtime: "node"}},
			"python": {{Name: "python", Version: "3.12.0", Description: "Python interpreter", Runtime: "python"}},
			"redis":  {{Name: "redis", Version: "7.2.0", Description: "Redis key-value store", Runtime: "redis"}},
		},
	}

	results := idx.Search("node")
	require.Len(t, results, 1)
	require.Equal(t, "node", results[0].Name)

	results = idx.Search("python")
	require.Len(t, results, 1)
	require.Equal(t, "python", results[0].Name)

	results = idx.Search("key")
	require.Len(t, results, 1)
	require.Equal(t, "redis", results[0].Name)

	results = idx.Search("nonexistent")
	require.Len(t, results, 0)
}

func TestIndex_Latest(t *testing.T) {
	idx := &Index{
		Packages: map[string][]Package{
			"node": {
				{Name: "node", Version: "20.11.0"},
				{Name: "node", Version: "18.19.0"},
			},
		},
	}

	pkg := idx.Latest("node")
	require.NotNil(t, pkg)
	require.Equal(t, "20.11.0", pkg.Version)

	pkg = idx.Latest("nonexistent")
	require.Nil(t, pkg)
}

func TestPackage_JSON(t *testing.T) {
	pkg := Package{
		Name:        "node",
		Version:     "20.11.0",
		Description: "Node.js runtime",
		Runtime:     "node",
		SHA256:      "abc123def456",
		Size:        42_000_000,
		URL:         "https://example.com/node-20.11.0.tar.gz",
	}
	data, err := json.MarshalIndent(pkg, "", "  ")
	require.NoError(t, err)
	require.Contains(t, string(data), `"name": "node"`)

	var decoded Package
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Equal(t, pkg.Name, decoded.Name)
	require.Equal(t, pkg.Version, decoded.Version)
	require.Equal(t, pkg.SHA256, decoded.SHA256)
}
