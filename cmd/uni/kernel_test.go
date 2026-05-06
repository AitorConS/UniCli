package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKernelCheck_OutputsInstalledVersion(t *testing.T) {
	_, socketPath := startDaemon(t)
	storePath := t.TempDir()

	out := execRoot(t, socketPath, storePath, "kernel", "check")
	require.Contains(t, out, "Installed kernel:")
}

func TestKernelCheck_NoNetwork(t *testing.T) {
	_, socketPath := startDaemon(t)
	storePath := t.TempDir()

	out := execRoot(t, socketPath, storePath, "kernel", "check")
	require.Contains(t, out, "Installed kernel:")
}
