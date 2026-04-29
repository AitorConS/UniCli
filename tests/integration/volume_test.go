//go:build integration

package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/AitorConS/unikernel-engine/internal/image"
	"github.com/AitorConS/unikernel-engine/internal/tools"
	"github.com/AitorConS/unikernel-engine/internal/vm"
	"github.com/AitorConS/unikernel-engine/internal/volume"
	"github.com/stretchr/testify/require"
)

// TestVolumePersistence verifies that data written to a virtio-blk volume
// survives a VM stop+restart cycle.
func TestVolumePersistence(t *testing.T) {
	requireQEMU(t)

	storeDir := t.TempDir()
	imgStore, err := image.NewStore(filepath.Join(storeDir, "images"))
	require.NoError(t, err)

	// Build the voltest binary for Linux.
	voltestBin := filepath.Join(t.TempDir(), "voltest")
	voltestSrc := filepath.Join("..", "..", "examples", "voltest", "main.go")
	require.NoError(t, buildLinuxBinary(voltestSrc, voltestBin), "failed to build voltest binary")

	// Build a real unikernel image using mkfs (auto-downloaded if needed).
	mkfsRun, err := tools.ResolveMkfs(context.Background(), filepath.Join(storeDir, "tools"), "")
	require.NoError(t, err, "failed to resolve mkfs")

	builder := image.NewBuilder(imgStore)
	_, err = builder.Build(context.Background(), image.BuildConfig{
		Name:       "voltest",
		Tag:        "latest",
		BinaryPath: voltestBin,
		MkfsRun:    mkfsRun,
		Memory:     "256M",
		CPUs:       1,
	})
	require.NoError(t, err, "failed to build voltest image")

	_, diskPath, err := imgStore.Get("voltest:latest")
	require.NoError(t, err)

	// Volume store.
	volStore, err := volume.NewStore(filepath.Join(storeDir, "volumes"))
	require.NoError(t, err)
	_, err = volStore.Create("testdata", 1<<30)
	require.NoError(t, err)
	vol, err := volStore.Get("testdata")
	require.NoError(t, err)

	mgr := vm.NewQEMUManager(defaultQEMU)
	srv, err := api.NewServer(mgr, defaultSocket, nil, "")
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	client := dialWithRetry(t, defaultSocket)
	defer func() { _ = client.Close() }()

	// First run: write data via HTTP.
	info1, err := client.Run(ctx, api.RunParams{
		ImagePath: diskPath,
		Memory:    "256M",
		CPUs:      1,
		PortMaps: []api.PortMapSpec{
			{HostPort: 18080, GuestPort: 8080, Protocol: "tcp"},
		},
		Volumes: []api.VolumeMountSpec{
			{DiskPath: vol.DiskPath, GuestPath: "/data"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, info1.ID)

	// Wait for the HTTP server inside the VM to be ready.
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://127.0.0.1:18080/")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 60*time.Second, 500*time.Millisecond, "voltest HTTP server did not become ready")

	// Write a message.
	resp, err := http.Post("http://127.0.0.1:18080/write?msg=hello", "", nil)
	require.NoError(t, err)
	_ = resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	// Stop and remove the first VM.
	require.NoError(t, client.Stop(ctx, info1.ID, false))
	require.Eventually(t, func() bool {
		g, err := client.Get(ctx, info1.ID)
		return err == nil && g.State == "stopped"
	}, 30*time.Second, 100*time.Millisecond)
	require.NoError(t, client.Remove(ctx, info1.ID))

	// Second run: same volume, new VM.
	info2, err := client.Run(ctx, api.RunParams{
		ImagePath: diskPath,
		Memory:    "256M",
		CPUs:      1,
		PortMaps: []api.PortMapSpec{
			{HostPort: 18080, GuestPort: 8080, Protocol: "tcp"},
		},
		Volumes: []api.VolumeMountSpec{
			{DiskPath: vol.DiskPath, GuestPath: "/data"},
		},
	})
	require.NoError(t, err)
	require.NotEmpty(t, info2.ID)

	// Wait for readiness again.
	require.Eventually(t, func() bool {
		resp, err := http.Get("http://127.0.0.1:18080/")
		if err != nil {
			return false
		}
		_ = resp.Body.Close()
		return resp.StatusCode == http.StatusOK
	}, 60*time.Second, 500*time.Millisecond, "voltest HTTP server did not become ready on second run")

	// Verify the previously written message is still there.
	resp, err = http.Get("http://127.0.0.1:18080/")
	require.NoError(t, err)
	body := make([]byte, 1024)
	n, _ := resp.Body.Read(body)
	_ = resp.Body.Close()
	require.Contains(t, string(body[:n]), "hello")

	// Cleanup.
	_ = client.Stop(ctx, info2.ID, false)
	require.Eventually(t, func() bool {
		g, err := client.Get(ctx, info2.ID)
		return err == nil && g.State == "stopped"
	}, 30*time.Second, 100*time.Millisecond)
	require.NoError(t, client.Remove(ctx, info2.ID))
}

// buildLinuxBinary compiles a Go source file into a static Linux ELF binary.
func buildLinuxBinary(src, dst string) error {
	cmd := exec.Command("go", "build", "-ldflags=-s -w", "-o", dst, src)
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH=amd64",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("build linux binary: %w (output: %s)", err, string(out))
	}
	return nil
}
