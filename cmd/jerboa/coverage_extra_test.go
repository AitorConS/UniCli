package main

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestNeedsDaemon(t *testing.T) {
	// Build a small command tree rooted at "jerboa" to mirror the real CLI: the
	// root and offline groups (config, kernel, …) need no daemon, while remote
	// verbs like "run" — and sign/verify, which resolve an image digest through
	// the daemon (F-019) — do.
	root := &cobra.Command{Use: "jerboa"}

	config := &cobra.Command{Use: "config"}
	configSet := &cobra.Command{Use: "set"}
	config.AddCommand(configSet)

	sign := &cobra.Command{Use: "sign"}
	verify := &cobra.Command{Use: "verify"}
	run := &cobra.Command{Use: "run"}
	root.AddCommand(config, sign, verify, run)

	require.False(t, needsDaemon(root), "root itself needs no daemon")
	require.False(t, needsDaemon(configSet), "offline group descendants need no daemon")
	require.False(t, needsDaemon(config), "offline group needs no daemon")
	require.True(t, needsDaemon(run), "remote verbs need the daemon")
	require.True(t, needsDaemon(sign), "sign resolves the image digest via the daemon (F-019)")
	require.True(t, needsDaemon(verify), "verify resolves the image digest via the daemon (F-019)")
}

func TestSigningStorePath(t *testing.T) {
	// Fake the home directory so we can assert the full resolved path rather than
	// a loose suffix (the old check passed for any path ending in ".jerboa",
	// including a bare "/.jerboa"). os.UserHomeDir reads USERPROFILE on Windows
	// and HOME elsewhere, so set both for portability.
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)

	require.Equal(t, home+"/.jerboa", signingStorePath(),
		"signing store must resolve to <home>/.jerboa")
}
