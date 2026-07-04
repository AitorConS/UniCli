package pkg

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// maliciousRefs are package coordinates a compromised remote index could
// return to escape the store root. Every store method that turns a coordinate
// into a filesystem path must reject them before touching disk.
var maliciousRefs = []struct {
	name  string
	value string
}{
	{"parent traversal", "../evil"},
	{"deep traversal", "../../../.ssh"},
	{"absolute-ish", "/etc/passwd"},
	{"backslash", `..\evil`},
	{"embedded dotdot", "a/../../b"},
	{"empty", ""},
}

func TestStore_RejectsMaliciousRefs(t *testing.T) {
	root := t.TempDir()
	store, err := NewStore(root)
	require.NoError(t, err)

	// A sentinel sibling of the store root that a traversal would try to reach.
	outside := filepath.Join(filepath.Dir(root), "SENTINEL")
	require.NoError(t, os.WriteFile(outside, []byte("do not touch"), 0o644))
	t.Cleanup(func() { _ = os.Remove(outside) })

	for _, m := range maliciousRefs {
		t.Run(m.name, func(t *testing.T) {
			// name field poisoned.
			require.Error(t, store.Download(Package{Name: m.value, Version: "1.0.0", URL: "http://127.0.0.1:0"}))
			require.Error(t, store.Extract(Package{Name: m.value, Version: "1.0.0"}))
			require.Error(t, store.SaveMeta(Package{Name: m.value, Version: "1.0.0"}))
			require.Error(t, store.Remove(m.value, "1.0.0"))
			require.Error(t, store.RemoveAll(m.value))
			require.Error(t, store.Create(m.value, "1.0.0", "bin", nil, "", ""))
			require.False(t, store.IsDownloaded(m.value, "1.0.0"))
			require.False(t, store.IsExtracted(m.value, "1.0.0"))
			_, err := store.ExtractedFiles(m.value, "1.0.0")
			require.Error(t, err)

			// version field poisoned.
			require.Error(t, store.Download(Package{Name: "node", Version: m.value, URL: "http://127.0.0.1:0"}))
			require.Error(t, store.Remove("node", m.value))
		})
	}

	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "do not touch", string(data), "traversal must not overwrite files outside the store root")
}

func TestOpsStore_RejectsMaliciousRefs(t *testing.T) {
	root := t.TempDir()
	store, err := NewOpsStore(root)
	require.NoError(t, err)

	outside := filepath.Join(filepath.Dir(root), "SENTINEL_OPS")
	require.NoError(t, os.WriteFile(outside, []byte("do not touch"), 0o644))
	t.Cleanup(func() { _ = os.Remove(outside) })

	for _, m := range maliciousRefs {
		t.Run(m.name, func(t *testing.T) {
			// Each of the three coordinates must be validated independently.
			require.Error(t, store.Download(m.value, "node", "v1.0.0", ""))
			require.Error(t, store.Download("eyberg", m.value, "v1.0.0", ""))
			require.Error(t, store.Download("eyberg", "node", m.value, ""))

			require.Error(t, store.Extract(m.value, "node", "v1.0.0"))
			require.Error(t, store.Remove(m.value, "node", "v1.0.0"))
			require.False(t, store.IsDownloaded(m.value, "node", "v1.0.0"))
			require.False(t, store.IsExtracted(m.value, "node", "v1.0.0"))

			_, err := store.ExtractedFiles(m.value, "node", "v1.0.0")
			require.Error(t, err)
			_, err = store.FindBinary(m.value, "node", "v1.0.0")
			require.Error(t, err)
			_, err = store.LoadPackageManifest(m.value, "node", "v1.0.0")
			require.Error(t, err)
		})
	}

	data, err := os.ReadFile(outside)
	require.NoError(t, err)
	require.Equal(t, "do not touch", string(data), "traversal must not overwrite files outside the store root")
}
