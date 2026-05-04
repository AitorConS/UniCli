package image

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestBuildManifest_NoPkgFiles(t *testing.T) {
	got := BuildManifest(filepath.FromSlash("/usr/bin/hello"), nil)
	require.Contains(t, got, "program:/program")
	require.Contains(t, got, "program:(contents:(host:")
	require.NotContains(t, got, "node")
}

func TestBuildManifest_WithPkgFiles(t *testing.T) {
	pkgFiles := []string{
		filepath.FromSlash("/home/user/.uni/packages/node/20.11.0/files/bin/node"),
		filepath.FromSlash("/home/user/.uni/packages/node/20.11.0/files/lib/libnode.so"),
	}
	got := BuildManifest(filepath.FromSlash("/usr/bin/hello"), pkgFiles)
	require.Contains(t, got, "program:/program")
	require.Contains(t, got, "node:(contents:(host:")
	require.Contains(t, got, "libnode.so:(contents:(host:")
}

func TestBuildManifest_PkgFilesIntegration(t *testing.T) {
	pkgDir := t.TempDir()
	binDir := filepath.Join(pkgDir, "bin")
	libDir := filepath.Join(pkgDir, "lib")
	require.NoError(t, os.MkdirAll(binDir, 0o755))
	require.NoError(t, os.MkdirAll(libDir, 0o755))

	binPath := filepath.Join(binDir, "myapp")
	libPath := filepath.Join(libDir, "libmyapp.so")
	require.NoError(t, os.WriteFile(binPath, []byte("binary"), 0o755))
	require.NoError(t, os.WriteFile(libPath, []byte("sharedlib"), 0o644))

	pkgFiles := []string{binPath, libPath}
	got := BuildManifest(binPath, pkgFiles)

	require.Contains(t, got, "myapp:(contents:(host:")
	require.Contains(t, got, "libmyapp.so:(contents:(host:")
	require.Contains(t, got, "program:/program")

	lines := strings.Count(got, ":(contents:(host:")
	require.Equal(t, 3, lines)
}