package image

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func makeDiskFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "disk-*.img")
	require.NoError(t, err)
	_, err = f.WriteString("fake disk image content")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func makeStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(filepath.Join(t.TempDir(), "images"))
	require.NoError(t, err)
	return s
}

func TestStore_Put_Get(t *testing.T) {
	s := makeStore(t)
	m := validManifest()
	disk := makeDiskFile(t)

	require.NoError(t, s.Put("hello", "latest", m, disk))

	got, diskPath, err := s.Get("hello:latest")
	require.NoError(t, err)
	require.Equal(t, "hello", got.Name)
	require.Equal(t, "latest", got.Tag)
	require.FileExists(t, diskPath)
}

func TestStore_Get_by_sha256(t *testing.T) {
	s := makeStore(t)
	m := validManifest()
	disk := makeDiskFile(t)
	require.NoError(t, s.Put("hello", "latest", m, disk))

	got, _, err := s.Get("hello:latest")
	require.NoError(t, err)

	// Get by full sha256 digest
	_, _, err = s.Get(got.DiskDigest)
	require.NoError(t, err)
}

func TestStore_Get_not_found(t *testing.T) {
	s := makeStore(t)
	_, _, err := s.Get("nonexistent:latest")
	require.Error(t, err)
}

func TestStore_List(t *testing.T) {
	s := makeStore(t)

	disk1 := makeDiskFile(t)
	disk2 := makeDiskFile(t)
	// Write different content so digests differ
	require.NoError(t, os.WriteFile(disk2, []byte("different content"), 0o644))

	require.NoError(t, s.Put("a", "latest", validManifest(), disk1))
	m2 := validManifest()
	m2.Name = "b"
	require.NoError(t, s.Put("b", "latest", m2, disk2))

	list, err := s.List()
	require.NoError(t, err)
	require.Len(t, list, 2)
}

func TestStore_List_empty(t *testing.T) {
	s := makeStore(t)
	list, err := s.List()
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestStore_Remove(t *testing.T) {
	s := makeStore(t)
	disk := makeDiskFile(t)
	require.NoError(t, s.Put("hello", "latest", validManifest(), disk))

	require.NoError(t, s.Remove("hello:latest"))

	list, err := s.List()
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestStore_Remove_not_found(t *testing.T) {
	s := makeStore(t)
	require.Error(t, s.Remove("nonexistent:latest"))
}

func TestStore_Multiple_tags_same_image(t *testing.T) {
	s := makeStore(t)
	disk := makeDiskFile(t)
	m := validManifest()

	require.NoError(t, s.Put("hello", "latest", m, disk))
	require.NoError(t, s.Put("hello", "v1.0", m, disk))

	list, err := s.List()
	require.NoError(t, err)
	// Same disk content → same sha256 → only one unique image in store
	require.Len(t, list, 1)

	// Remove one tag — image dir should remain (other tag still points to it).
	require.NoError(t, s.Remove("hello:v1.0"))
	_, _, err = s.Get("hello:latest")
	require.NoError(t, err)
}
