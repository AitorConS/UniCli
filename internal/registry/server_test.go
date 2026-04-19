package registry_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AitorConS/unikernel-engine/internal/image"
	"github.com/AitorConS/unikernel-engine/internal/registry"
	"github.com/stretchr/testify/require"
)

func makeStore(t *testing.T) *image.Store {
	t.Helper()
	s, err := image.NewStore(filepath.Join(t.TempDir(), "images"))
	require.NoError(t, err)
	return s
}

func makeDiskFile(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "disk-*.img")
	require.NoError(t, err)
	_, err = f.WriteString("fake disk image content for registry test")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}

func seedStore(t *testing.T, s *image.Store) image.Manifest {
	t.Helper()
	disk := makeDiskFile(t)
	m := image.Manifest{
		SchemaVersion: image.SchemaVersion,
		Name:          "hello",
		Tag:           "latest",
		Created:       time.Now().UTC(),
		Config:        image.Config{Memory: "256M", CPUs: 1},
		DiskDigest:    "sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		DiskSize:      1024,
	}
	require.NoError(t, s.Put("hello", "latest", m, disk))
	got, _, err := s.Get("hello:latest")
	require.NoError(t, err)
	return got
}

func startServer(t *testing.T) (*httptest.Server, *image.Store) {
	t.Helper()
	store := makeStore(t)
	srv := httptest.NewServer(registry.NewServer(store).Handler())
	t.Cleanup(srv.Close)
	return srv, store
}

func TestServer_List_empty(t *testing.T) {
	srv, _ := startServer(t)
	client := registry.NewClient(srv.URL)

	list, err := client.List(context.Background())
	require.NoError(t, err)
	require.Empty(t, list)
}

func TestServer_Push_and_List(t *testing.T) {
	srv, _ := startServer(t)
	client := registry.NewClient(srv.URL)

	srcStore := makeStore(t)
	m := seedStore(t, srcStore)
	_, diskPath, err := srcStore.Get("hello:latest")
	require.NoError(t, err)

	require.NoError(t, client.Push(context.Background(), m, diskPath))

	list, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, list, 1)
	require.Equal(t, "hello", list[0].Name)
}

func TestServer_Pull(t *testing.T) {
	srv, srvStore := startServer(t)
	client := registry.NewClient(srv.URL)

	// Seed the server-side store directly.
	disk := makeDiskFile(t)
	m := image.Manifest{
		SchemaVersion: image.SchemaVersion,
		Name:          "hello",
		Tag:           "latest",
		Created:       time.Now().UTC(),
		Config:        image.Config{Memory: "256M", CPUs: 1},
		DiskDigest:    "sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		DiskSize:      1024,
	}
	require.NoError(t, srvStore.Put("hello", "latest", m, disk))

	// Pull into a separate local store.
	localStore := makeStore(t)
	pulled, err := client.Pull(context.Background(), "hello:latest", localStore)
	require.NoError(t, err)
	require.Equal(t, "hello", pulled.Name)

	_, diskPath, err := localStore.Get("hello:latest")
	require.NoError(t, err)
	require.FileExists(t, diskPath)
}

func TestServer_Push_Pull_roundtrip(t *testing.T) {
	srv, _ := startServer(t)
	client := registry.NewClient(srv.URL)

	// Build source image.
	srcStore := makeStore(t)
	disk := makeDiskFile(t)
	m := image.Manifest{
		SchemaVersion: image.SchemaVersion,
		Name:          "myapp",
		Tag:           "v1",
		Created:       time.Now().UTC(),
		Config:        image.Config{Memory: "512M", CPUs: 2},
		DiskDigest:    "sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		DiskSize:      1024,
	}
	require.NoError(t, srcStore.Put("myapp", "v1", m, disk))
	pushed, _, err := srcStore.Get("myapp:v1")
	require.NoError(t, err)
	_, diskPath, err := srcStore.Get("myapp:v1")
	require.NoError(t, err)

	require.NoError(t, client.Push(context.Background(), pushed, diskPath))

	// Pull into clean store.
	dstStore := makeStore(t)
	pulled, err := client.Pull(context.Background(), "myapp:v1", dstStore)
	require.NoError(t, err)
	require.Equal(t, "myapp", pulled.Name)
	require.Equal(t, "v1", pulled.Tag)
}
