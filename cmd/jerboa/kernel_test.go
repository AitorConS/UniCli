//go:build linux

package main

import (
	"os"
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

func TestConfirmPrompt(t *testing.T) {
	orig := os.Stdin
	t.Cleanup(func() { os.Stdin = orig })

	r, w, err := os.Pipe()
	require.NoError(t, err)
	_, err = w.WriteString("yes\n")
	require.NoError(t, err)
	require.NoError(t, w.Close())
	os.Stdin = r

	require.True(t, confirmPrompt("Proceed? "))
}
