package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/AitorConS/unikernel-engine/internal/api"
	"github.com/AitorConS/unikernel-engine/internal/compose"
	"github.com/AitorConS/unikernel-engine/internal/vm"
	"github.com/stretchr/testify/require"
)

var testComposeYAML = []byte(`
version: "1"
services:
  backend:
    image: DISK_PATH
    memory: 256M
  frontend:
    image: DISK_PATH
    memory: 256M
    depends_on:
      - backend
`)

// writeComposeFile writes a compose YAML with the given disk path substituted.
func writeComposeFile(t *testing.T, diskPath string) string {
	t.Helper()
	dir := t.TempDir()
	content := strings.ReplaceAll(string(testComposeYAML), "DISK_PATH", filepath.ToSlash(diskPath))
	path := filepath.Join(dir, "uni-compose.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}

func startComposeDaemon(t *testing.T) (*api.Client, string) {
	t.Helper()
	socketPath := filepath.Join(t.TempDir(), "unid.sock")
	mgr := vm.NewQEMUManager("fake-qemu", vm.WithCommandFunc(fakeQEMUCmd()))
	srv, err := api.NewServer(mgr, socketPath, nil, "")
	require.NoError(t, err)

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = srv.Serve(ctx) }()
	t.Cleanup(cancel)

	var client *api.Client
	require.Eventually(t, func() bool {
		var dialErr error
		client, dialErr = api.Dial(socketPath)
		return dialErr == nil
	}, testTimeout, dialPoll)
	t.Cleanup(func() { _ = client.Close() })
	return client, socketPath
}

func TestComposeUp_StartsServices(t *testing.T) {
	client, socketPath := startComposeDaemon(t)
	storePath := t.TempDir()

	diskPath := filepath.Join(t.TempDir(), "disk.img")
	require.NoError(t, os.WriteFile(diskPath, []byte("fake"), 0o600))

	composeFile := writeComposeFile(t, diskPath)

	out := execRoot(t, socketPath, storePath, "compose", "up", composeFile)
	require.Contains(t, out, "started backend")
	require.Contains(t, out, "started frontend")

	// state file should exist
	stateData, err := os.ReadFile(filepath.Join(filepath.Dir(composeFile), stateFileName))
	require.NoError(t, err)
	require.Contains(t, string(stateData), "backend")

	// verify VMs are running via daemon
	infos, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, infos, 2)
}

func TestComposePs(t *testing.T) {
	_, socketPath := startComposeDaemon(t)
	storePath := t.TempDir()

	diskPath := filepath.Join(t.TempDir(), "disk.img")
	require.NoError(t, os.WriteFile(diskPath, []byte("fake"), 0o600))
	composeFile := writeComposeFile(t, diskPath)

	execRoot(t, socketPath, storePath, "compose", "up", composeFile)

	out := execRoot(t, socketPath, storePath, "compose", "ps", composeFile)
	require.Contains(t, out, "backend")
	require.Contains(t, out, "frontend")
	require.Contains(t, out, "running")
}

func TestComposePs_JSON(t *testing.T) {
	_, socketPath := startComposeDaemon(t)
	storePath := t.TempDir()

	diskPath := filepath.Join(t.TempDir(), "disk.img")
	require.NoError(t, os.WriteFile(diskPath, []byte("fake"), 0o600))
	composeFile := writeComposeFile(t, diskPath)

	execRoot(t, socketPath, storePath, "compose", "up", composeFile)

	out := execRoot(t, socketPath, storePath, "--output", "json", "compose", "ps", composeFile)
	require.Contains(t, out, `"service"`)
}

func TestComposeLogs(t *testing.T) {
	_, socketPath := startComposeDaemon(t)
	storePath := t.TempDir()

	diskPath := filepath.Join(t.TempDir(), "disk.img")
	require.NoError(t, os.WriteFile(diskPath, []byte("fake"), 0o600))
	composeFile := writeComposeFile(t, diskPath)

	execRoot(t, socketPath, storePath, "compose", "up", composeFile)
	_ = execRoot(t, socketPath, storePath, "compose", "logs", composeFile, "backend")
}

func TestComposeLogs_UnknownService(t *testing.T) {
	_, socketPath := startComposeDaemon(t)
	storePath := t.TempDir()

	diskPath := filepath.Join(t.TempDir(), "disk.img")
	require.NoError(t, os.WriteFile(diskPath, []byte("fake"), 0o600))
	composeFile := writeComposeFile(t, diskPath)

	execRoot(t, socketPath, storePath, "compose", "up", composeFile)

	msg := execRootExpectError(t, socketPath, storePath, "compose", "logs", composeFile, "unknown")
	require.Contains(t, msg, "not found")
}

func TestComposeDown_StopsServices(t *testing.T) {
	client, socketPath := startComposeDaemon(t)
	storePath := t.TempDir()

	diskPath := filepath.Join(t.TempDir(), "disk.img")
	require.NoError(t, os.WriteFile(diskPath, []byte("fake"), 0o600))
	composeFile := writeComposeFile(t, diskPath)

	execRoot(t, socketPath, storePath, "compose", "up", composeFile)

	infos, err := client.List(context.Background())
	require.NoError(t, err)
	require.Len(t, infos, 2)

	execRoot(t, socketPath, storePath, "compose", "down", "--force", composeFile)

	// state file should be removed
	_, statErr := os.Stat(filepath.Join(filepath.Dir(composeFile), stateFileName))
	require.True(t, os.IsNotExist(statErr))
}

func TestComposeDown_NoState(t *testing.T) {
	_, socketPath := startComposeDaemon(t)
	storePath := t.TempDir()

	composeFile := filepath.Join(t.TempDir(), "uni-compose.yaml")
	require.NoError(t, os.WriteFile(composeFile, testComposeYAML, 0o600))

	msg := execRootExpectError(t, socketPath, storePath, "compose", "down", composeFile)
	require.Contains(t, msg, "compose down")
}

func TestComposeUp_InvalidFile(t *testing.T) {
	_, socketPath := startComposeDaemon(t)
	storePath := t.TempDir()

	badFile := filepath.Join(t.TempDir(), "bad.yaml")
	require.NoError(t, os.WriteFile(badFile, []byte(`version: "1"\nservices:`), 0o600))

	msg := execRootExpectError(t, socketPath, storePath, "compose", "up", badFile)
	require.Contains(t, msg, "compose up")
}

func TestComposeUpWithCtx(t *testing.T) {
	client, _ := startComposeDaemon(t)
	storePath := t.TempDir()

	diskPath := filepath.Join(t.TempDir(), "disk.img")
	require.NoError(t, os.WriteFile(diskPath, []byte("fake"), 0o600))

	f := compose.File{
		Version: "1",
		Services: map[string]compose.Service{
			"svc": {Image: diskPath, Memory: "256M"},
		},
	}
	state, err := composeUpWithCtx(context.Background(), client, f, storePath)
	require.NoError(t, err)
	require.NotEmpty(t, state.Services["svc"])
}

func TestStateServiceNames_Sorted(t *testing.T) {
	state := compose.State{
		Services: map[string]string{"z": "id1", "a": "id2", "m": "id3"},
	}
	names := stateServiceNames(state)
	require.Equal(t, []string{"a", "m", "z"}, names)
}
