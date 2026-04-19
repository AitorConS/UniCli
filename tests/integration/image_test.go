//go:build integration

package integration

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/AitorConS/unikernel-engine/internal/image"
	"github.com/AitorConS/unikernel-engine/internal/registry"
	"github.com/stretchr/testify/require"
)

// TestImageRoundtrip tests build → push → pull round-trip without QEMU.
// "Build" here seeds the store directly since mkfs requires the kernel build.
func TestImageRoundtrip(t *testing.T) {
	disk := makeTmpDisk(t)

	srcStore, err := image.NewStore(filepath.Join(t.TempDir(), "src-store"))
	require.NoError(t, err)

	m := image.Manifest{
		SchemaVersion: image.SchemaVersion,
		Name:          "hello",
		Tag:           "latest",
		Config:        image.Config{Memory: "256M", CPUs: 1},
		DiskDigest:    "sha256:abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		DiskSize:      1024,
	}
	require.NoError(t, srcStore.Put("hello", "latest", m, disk))

	// Start registry server.
	srvStore, err := image.NewStore(filepath.Join(t.TempDir(), "registry-store"))
	require.NoError(t, err)
	ts := httptest.NewServer(registry.NewServer(srvStore).Handler())
	defer ts.Close()

	client := registry.NewClient(ts.URL)
	ctx := context.Background()

	// Push
	pushed, _, err := srcStore.Get("hello:latest")
	require.NoError(t, err)
	_, diskPath, err := srcStore.Get("hello:latest")
	require.NoError(t, err)
	require.NoError(t, client.Push(ctx, pushed, diskPath))

	// Pull into clean store.
	dstStore, err := image.NewStore(filepath.Join(t.TempDir(), "dst-store"))
	require.NoError(t, err)
	pulled, err := client.Pull(ctx, "hello:latest", dstStore)
	require.NoError(t, err)
	require.Equal(t, "hello", pulled.Name)
	require.Equal(t, "latest", pulled.Tag)

	// Verify disk image exists in destination store.
	_, dstDisk, err := dstStore.Get("hello:latest")
	require.NoError(t, err)
	require.FileExists(t, dstDisk)

	// List from registry.
	list, err := client.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
}

func makeTmpDisk(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "disk-*.img")
	require.NoError(t, err)
	_, err = f.WriteString("fake disk image content for integration test")
	require.NoError(t, err)
	require.NoError(t, f.Close())
	return f.Name()
}
