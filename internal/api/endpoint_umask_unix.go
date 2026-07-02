//go:build unix

package api

import "syscall"

func restrictUnixSocketUmask() func() {
	old := syscall.Umask(0o177)
	return func() {
		syscall.Umask(old)
	}
}
