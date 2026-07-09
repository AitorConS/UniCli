package wsldistro

import (
	"bytes"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseDistroList_UTF16(t *testing.T) {
	// wsl --list --quiet emits UTF-16LE (each ASCII char followed by a NUL),
	// CRLF line endings, and a trailing blank line.
	utf16 := func(s string) []byte {
		var b []byte
		for _, r := range s {
			b = append(b, byte(r), 0)
		}
		return b
	}
	raw := utf16("Ubuntu\r\njerboa\r\ndocker-desktop\r\n\r\n")

	got := parseDistroList(raw)
	require.Equal(t, []string{"Ubuntu", "jerboa", "docker-desktop"}, got)
}

func TestParseDistroList_Empty(t *testing.T) {
	require.Empty(t, parseDistroList(nil))
}

func TestDecodeWSLOutput_StripsNUL(t *testing.T) {
	// UTF-16LE "hi" plus a CR is decoded back to ASCII.
	require.Equal(t, "hi\r", decodeWSLOutput([]byte{'h', 0, 'i', 0, '\r', 0}))
}

func TestInstallDaemonBinary_OpenError(t *testing.T) {
	// A missing source binary fails before any `wsl` invocation, so the open
	// error path is exercised without a real distro.
	err := InstallDaemonBinary(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "wsldistro: open")
}

func TestDataDirs_ExcludesToolsCache(t *testing.T) {
	// The kernel toolchain cache re-downloads on demand, so it must never be in
	// the preserved set; the user-data dirs must be.
	require.Equal(t, []string{"images", "vms", "networks"}, DataDirs)
	require.NotContains(t, DataDirs, "tools")
}

func TestExportImportData_WrapErrorsWithoutWSL(t *testing.T) {
	// On the Linux CI there is no `wsl`, so both calls fail at exec; assert they
	// wrap the error under the package prefix rather than panicking. On Windows
	// they would drive the real distro, so skip there.
	if runtime.GOOS == "windows" {
		t.Skip("on Windows these drive the real WSL distro")
	}
	var buf bytes.Buffer
	err := ExportData(&buf)
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "wsldistro: export data"), err.Error())

	err = ImportData(strings.NewReader("ignored"))
	require.Error(t, err)
	require.True(t, strings.HasPrefix(err.Error(), "wsldistro: import data"), err.Error())
}

func TestDefaultInstallDir_FallbackHome(t *testing.T) {
	t.Setenv("LOCALAPPDATA", "")
	require.Contains(t, DefaultInstallDir(), filepath.Join(".jerboa", "distro"))
}

func TestDefaultInstallDir_UsesLocalAppData(t *testing.T) {
	base := `C:\Users\test\AppData\Local`
	t.Setenv("LOCALAPPDATA", base)
	// filepath.Join uses the host separator, so build the expectation the same
	// way to keep the test portable across the Linux CI and Windows.
	require.Equal(t, filepath.Join(base, "jerboa", "distro"), DefaultInstallDir())
}
