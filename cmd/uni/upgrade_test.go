package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCliParseSemver(t *testing.T) {
	tests := []struct {
		input string
		want  [3]int
	}{
		{"v0.1.0", [3]int{0, 1, 0}},
		{"0.1.0", [3]int{0, 1, 0}},
		{"v1.2.3", [3]int{1, 2, 3}},
		{"", [3]int{0, 0, 0}},
		{"1", [3]int{1, 0, 0}},
		{"bad", [3]int{0, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := cliParseSemver(tt.input)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestCliSemverGT(t *testing.T) {
	require.True(t, cliSemverGT("v0.2.0", "v0.1.0"))
	require.True(t, cliSemverGT("v1.0.0", "v0.99.99"))
	require.False(t, cliSemverGT("v0.1.0", "v0.1.0"))
	require.False(t, cliSemverGT("v0.1.0", "v0.2.0"))
}

func TestCliIsNewer(t *testing.T) {
	require.True(t, cliIsNewer("v0.1.0", "v0.2.0"))
	require.False(t, cliIsNewer("v0.2.0", "v0.1.0"))
	require.False(t, cliIsNewer("v0.1.0", "v0.1.0"))
}

func TestBinaryName(t *testing.T) {
	got := binaryName("uni")
	if runtime.GOOS == "windows" {
		require.Equal(t, "uni.exe", got)
	} else {
		require.Equal(t, "uni", got)
	}
}

func TestBinaryExt(t *testing.T) {
	got := binaryExt()
	if runtime.GOOS == "windows" {
		require.Equal(t, ".exe", got)
	} else {
		require.Empty(t, got)
	}
}

func TestCleanupBackups(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uni.bak"), []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unid.bak"), []byte("old"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "uni"), []byte("new"), 0o755))

	cleanupBackups(dir)

	_, err := os.Stat(filepath.Join(dir, "uni.bak"))
	require.True(t, os.IsNotExist(err), "uni.bak should be deleted")

	_, err = os.Stat(filepath.Join(dir, "unid.bak"))
	require.True(t, os.IsNotExist(err), "unid.bak should be deleted")

	_, err = os.Stat(filepath.Join(dir, "uni"))
	require.NoError(t, err, "uni should still exist")
}

func TestInstallBinary(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "new-binary")
	dest := filepath.Join(dir, "final-binary")

	require.NoError(t, os.WriteFile(src, []byte("content"), 0o755))
	require.NoError(t, installBinary(src, dest))

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	require.Equal(t, "content", string(got))

	_, err = os.Stat(src)
	require.True(t, os.IsNotExist(err), "source should be renamed away")
}

func TestDownloadTo_ServerError(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var buf bytes.Buffer
	err := downloadTo(ctx, "http://127.0.0.1:0/nonexistent", &buf)
	require.Error(t, err)
}
