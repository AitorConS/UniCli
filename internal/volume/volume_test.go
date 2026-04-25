package volume

import (
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStore_CreateAndGet(t *testing.T) {
	dir := t.TempDir()
	store, err := NewStore(dir)
	require.NoError(t, err)

	v, err := store.Create("data", 1<<20) // 1 MiB
	require.NoError(t, err)
	require.Equal(t, "data", v.ID)
	require.Equal(t, int64(1<<20), v.SizeBytes)
	require.FileExists(t, v.DiskPath)

	got, err := store.Get("data")
	require.NoError(t, err)
	require.Equal(t, "data", got.ID)
}

func TestStore_Create_duplicate(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.Create("dup", 0)
	require.NoError(t, err)
	_, err = store.Create("dup", 0)
	require.Error(t, err)
}

func TestStore_Create_default_size(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	v, err := store.Create("default", 0)
	require.NoError(t, err)
	require.Equal(t, int64(defaultSizeBytes), v.SizeBytes)
}

func TestStore_Get_notfound(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.Get("missing")
	require.Error(t, err)
}

func TestStore_List(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	_, err = store.Create("vol1", 1<<20)
	require.NoError(t, err)
	_, err = store.Create("vol2", 1<<20)
	require.NoError(t, err)

	vols, err := store.List()
	require.NoError(t, err)
	require.Len(t, vols, 2)
}

func TestStore_Remove(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	v, err := store.Create("todel", 1<<20)
	require.NoError(t, err)
	diskPath := v.DiskPath

	require.NoError(t, store.Remove("todel"))

	_, err = store.Get("todel")
	require.Error(t, err)
	require.NoFileExists(t, diskPath)
}

func TestStore_Remove_notfound(t *testing.T) {
	store, err := NewStore(t.TempDir())
	require.NoError(t, err)

	require.Error(t, store.Remove("ghost"))
}

func TestAllocateDisk_creates_file_of_correct_size(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "disk*.img")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	const sz = 4096
	require.NoError(t, allocateDisk(f.Name(), sz))

	info, err := os.Stat(f.Name())
	require.NoError(t, err)
	require.Equal(t, int64(sz), info.Size())
}

func TestParseSize(t *testing.T) {
	cases := []struct {
		input string
		want  int64
		isErr bool
	}{
		{"1G", 1 << 30, false},
		{"512M", 512 << 20, false},
		{"2048K", 2048 << 10, false},
		{"1073741824", 1073741824, false},
		{"", 0, false},
		{"bad", 0, true},
		{"1X", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			got, err := ParseSize(tc.input)
			if tc.isErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
