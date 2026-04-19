package api_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/AitorConS/unikernel-engine/internal/vm"
	"github.com/stretchr/testify/require"
)

func startTestServer(t *testing.T) (*api.Client, context.CancelFunc) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "unid.sock")
	mgr := vm.NewQEMUManager("fake-qemu", vm.WithCommandFunc(func(_ string, _ ...string) *exec.Cmd {
		return exec.Command("sleep", "30")
	}))
	srv, err := api.NewServer(mgr, socketPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := srv.Serve(ctx); err != nil {
			t.Logf("server stopped: %v", err)
		}
	}()

	var client *api.Client
	require.Eventually(t, func() bool {
		var dialErr error
		client, dialErr = api.Dial(socketPath)
		return dialErr == nil
	}, 2*time.Second, 10*time.Millisecond, "server did not start")

	t.Cleanup(func() {
		_ = client.Close()
		cancel()
	})
	return client, cancel
}

func TestServer_Run(t *testing.T) {
	client, _ := startTestServer(t)

	info, err := client.Run(context.Background(), api.RunParams{
		ImagePath: "test.img",
		Memory:    "256M",
		CPUs:      1,
	})
	require.NoError(t, err)
	require.NotEmpty(t, info.ID)
	require.Equal(t, "running", info.State)
}

func TestServer_List(t *testing.T) {
	client, _ := startTestServer(t)

	_, err := client.Run(context.Background(), api.RunParams{ImagePath: "a.img", Memory: "256M"})
	require.NoError(t, err)
	_, err = client.Run(context.Background(), api.RunParams{ImagePath: "b.img", Memory: "256M"})
	require.NoError(t, err)

	infos, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, infos, 2)
}

func TestServer_Get(t *testing.T) {
	client, _ := startTestServer(t)

	info, err := client.Run(context.Background(), api.RunParams{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)

	got, err := client.Get(context.Background(), info.ID)
	require.NoError(t, err)
	require.Equal(t, info.ID, got.ID)
}

func TestServer_Stop(t *testing.T) {
	client, _ := startTestServer(t)

	info, err := client.Run(context.Background(), api.RunParams{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)

	require.NoError(t, client.Stop(context.Background(), info.ID, false))

	require.Eventually(t, func() bool {
		got, err := client.Get(context.Background(), info.ID)
		return err == nil && got.State == "stopped"
	}, 5*time.Second, 50*time.Millisecond, "VM did not reach stopped state")
}

func TestServer_UnknownMethod(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "unid.sock")
	mgr := vm.NewQEMUManager("fake-qemu")
	srv, err := api.NewServer(mgr, socketPath)
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()

	var client *api.Client
	require.Eventually(t, func() bool {
		var dialErr error
		client, dialErr = api.Dial(socketPath)
		return dialErr == nil
	}, 2*time.Second, 10*time.Millisecond)
	defer func() { _ = client.Close() }()

	_, err = client.Get(context.Background(), "nonexistent")
	require.Error(t, err)
}
