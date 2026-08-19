package main

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestWarnMangledImagePath is the F-013/F-025 regression: an in-image path arg
// that arrived as a Windows drive path (Git-Bash/MSYS mangling of "/db") triggers
// a helpful warning, while a genuine POSIX in-image path does not.
func TestWarnMangledImagePath(t *testing.T) {
	for _, mangled := range []string{`C:\Program Files\Git\db`, "C:/Program Files/Git/usr/local/bin/redis-server", "D:/x"} {
		var b bytes.Buffer
		require.True(t, warnMangledImagePath(&b, "--src", mangled), mangled)
		require.Contains(t, b.String(), "MSYS_NO_PATHCONV")
	}
	for _, ok := range []string{"/db", "/usr/local/bin/redis-server", "/", ""} {
		var b bytes.Buffer
		require.False(t, warnMangledImagePath(&b, "--src", ok), ok)
		require.Empty(t, b.String())
	}
}
