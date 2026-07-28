package pkg

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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

// TestStore_Extract_RejectsTraversal checks the Zip Slip guard: an entry whose
// name climbs out of the extraction directory aborts the extraction instead of
// writing next to it. Rooted names are re-rooted by filepath.Join and stay in.
func TestStore_Extract_RejectsTraversal(t *testing.T) {
	for _, entry := range []string{"../escaped", "sub/../../escaped", "a/b/../../../escaped"} {
		t.Run(entry, func(t *testing.T) {
			dir := t.TempDir()
			store, err := NewStore(dir)
			require.NoError(t, err)

			pkgDir := store.PackageDir("evilpkg", "1.0.0")
			require.NoError(t, os.MkdirAll(pkgDir, 0o755))

			f, err := os.Create(filepath.Join(pkgDir, "files.tar.gz"))
			require.NoError(t, err)
			gw := gzip.NewWriter(f)
			tw := tar.NewWriter(gw)
			require.NoError(t, tw.WriteHeader(&tar.Header{
				Name: entry, Typeflag: tar.TypeReg, Size: 5, Mode: 0o644,
			}))
			_, err = tw.Write([]byte("pwned"))
			require.NoError(t, err)
			require.NoError(t, tw.Close())
			require.NoError(t, gw.Close())
			require.NoError(t, f.Close())

			err = store.Extract(Package{Name: "evilpkg", Version: "1.0.0"})
			require.Error(t, err)
			require.Contains(t, err.Error(), "escapes extraction directory")
			_, statErr := os.Stat(filepath.Join(pkgDir, "escaped"))
			require.True(t, os.IsNotExist(statErr), "traversing entry must not be written")
		})
	}
}

// TestExtract_SymlinkEscape covers the full archive path: a symlink pointing
// outside the package dir followed by an entry writing "through" it. The entry
// name stays lexically inside the package, so only the link check and the
// symlink-aware path walk keep the write contained.
func TestExtract_SymlinkEscape(t *testing.T) {
	root := t.TempDir()
	store, err := NewOpsStore(root)
	require.NoError(t, err)

	dir := store.PackageDir("evil", "pkg", "1.0")
	require.NoError(t, os.MkdirAll(dir, 0o755))

	outsideDir := t.TempDir()
	victim := filepath.Join(outsideDir, "authorized_keys")
	require.NoError(t, os.WriteFile(victim, []byte("original"), 0o600))

	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "pkg/", Typeflag: tar.TypeDir, Mode: 0o755,
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "pkg/sysroot", Typeflag: tar.TypeSymlink, Linkname: outsideDir, Mode: 0o777,
	}))
	require.NoError(t, tw.WriteHeader(&tar.Header{
		Name: "pkg/sysroot/authorized_keys", Typeflag: tar.TypeReg, Mode: 0o644, Size: int64(len("pwned")),
	}))
	_, err = tw.Write([]byte("pwned"))
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())

	require.NoError(t, os.WriteFile(filepath.Join(dir, ArchSlug()+".tar.gz"), buf.Bytes(), 0o644))
	require.NoError(t, store.Extract("evil", "pkg", "1.0"))

	got, err := os.ReadFile(victim)
	require.NoError(t, err)
	require.Equal(t, "original", string(got), "extraction must not write through an escaping symlink")
}

// TestTraversesSymlink covers the second layer of the extraction guard: even if
// a link were created inside the package dir, an entry whose path crosses it is
// refused, because os.OpenFile and os.MkdirAll would follow it off the disk.
func TestTraversesSymlink(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sysroot", "lib"), 0o755))
	if err := os.Symlink(outside, filepath.Join(dir, "escape")); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}

	require.False(t, traversesSymlink(dir, filepath.Join(dir, "sysroot", "lib", "libc.so")))
	require.False(t, traversesSymlink(dir, filepath.Join(dir, "not", "created", "yet")))
	require.True(t, traversesSymlink(dir, filepath.Join(dir, "escape")))
	require.True(t, traversesSymlink(dir, filepath.Join(dir, "escape", "authorized_keys")))
}
