package api_test

import (
	"context"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/AitorConS/unikernel-engine/internal/vm"
	"github.com/stretchr/testify/require"
)

// fakeQEMUCmd returns a vm.CommandFunc suitable for tests.
func fakeQEMUCmd(block bool) vm.CommandFunc {
	return func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		if block {
			if runtime.GOOS == "windows" {
				return exec.Command("powershell", "-Command", "while ($true) { Start-Sleep -Seconds 3600 }")
			}
			return exec.Command("sleep", "3600")
		}
		if runtime.GOOS == "windows" {
			return exec.Command("cmd", "/c", "exit 0")
		}
		return exec.Command("true")
	}
}

func startTestServer(t *testing.T) (*api.Client, context.CancelFunc) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "unid.sock")
	mgr := vm.NewQEMUManager("fake-qemu", vm.WithCommandFunc(fakeQEMUCmd(true)))
	srv, err := api.NewServer(mgr, socketPath, nil, "")
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

func TestServer_Run_WithPortsAndEnv(t *testing.T) {
	client, _ := startTestServer(t)

	info, err := client.Run(context.Background(), api.RunParams{
		ImagePath: "test.img",
		Memory:    "256M",
		CPUs:      1,
		Name:      "myvm",
		Env:       []string{"FOO=bar", "PORT=8080"},
		PortMaps: []api.PortMapSpec{
			{HostPort: 8080, GuestPort: 80, Protocol: "tcp"},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "myvm", info.Name)

	detail, err := client.Inspect(context.Background(), info.ID)
	require.NoError(t, err)
	require.Equal(t, "myvm", detail.Name)
	require.Equal(t, []string{"FOO=bar", "PORT=8080"}, detail.Env)
	require.Len(t, detail.Ports, 1)
	require.Equal(t, uint16(8080), detail.Ports[0].HostPort)
	require.Equal(t, uint16(80), detail.Ports[0].GuestPort)
	require.Equal(t, "tcp", detail.Ports[0].Protocol)
}

func TestServer_Run_AutoRemove(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "unid.sock")
	mgr := vm.NewQEMUManager("fake-qemu", vm.WithCommandFunc(fakeQEMUCmd(false)))
	srv, err := api.NewServer(mgr, socketPath, nil, "")
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

	info, err := client.Run(context.Background(), api.RunParams{
		ImagePath:  "test.img",
		Memory:     "256M",
		AutoRemove: true,
	})
	require.NoError(t, err)

	// With auto-remove and a process that exits immediately, the VM should
	// eventually disappear from the list.
	require.Eventually(t, func() bool {
		infos, _ := client.List(context.Background())
		for _, v := range infos {
			if v.ID == info.ID {
				return false
			}
		}
		return true
	}, 5*time.Second, 50*time.Millisecond, "VM was not auto-removed")
}

func TestServer_UnknownMethod(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "unid.sock")
	mgr := vm.NewQEMUManager("fake-qemu")
	srv, err := api.NewServer(mgr, socketPath, nil, "")
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
