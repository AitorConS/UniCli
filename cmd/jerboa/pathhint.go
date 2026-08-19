package main

import (
	"fmt"
	"io"
	"regexp"
)

// winDrivePathRe matches an absolute Windows path (a drive letter followed by a
// separator), e.g. C:\foo or D:/bar.
var winDrivePathRe = regexp.MustCompile(`^[A-Za-z]:[\\/]`)

// warnMangledImagePath warns when a flag that must carry a path INSIDE the image
// or container (an absolute POSIX path like /db or /usr/local/bin/redis-server)
// instead received a Windows drive path. That is the signature of Git-Bash/MSYS
// rewriting a leading "/foo" into "C:/Program Files/Git/foo" before jerboa ever
// sees the argument (E2E findings F-013/F-025) — a baffling failure otherwise. It
// returns true when it emitted a warning.
func warnMangledImagePath(w io.Writer, flag, value string) bool {
	if !winDrivePathRe.MatchString(value) {
		return false
	}
	fmt.Fprintf(w, "warning: %s=%q looks like a Windows path, but it must be a path INSIDE the image/container (e.g. /db).\n", flag, value)
	fmt.Fprintln(w, "         Git Bash (MSYS) rewrites a leading \"/...\" into a Windows path before jerboa sees it.")
	fmt.Fprintln(w, "         Re-run from PowerShell, or set MSYS_NO_PATHCONV=1 (or MSYS2_ARG_CONV_EXCL='*') in Git Bash.")
	return true
}
