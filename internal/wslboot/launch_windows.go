//go:build windows

package wslboot

import "syscall"

// Windows process creation flags for the short-lived `wsl` launcher.
const (
	createNewProcessGroup = 0x00000200
	createNoWindow        = 0x08000000
)

// detachAttr runs the `wsl` launcher with a hidden console. CREATE_NO_WINDOW
// (not DETACHED_PROCESS: CreateProcess ignores CREATE_NO_WINDOW when combined
// with it, which used to pop a Windows Terminal window) keeps the launch
// invisible; the new process group shields it from the client's Ctrl+C. The
// daemon itself survives via setsid inside the distro, so the launcher no
// longer needs to outlive the client.
func detachAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		CreationFlags: createNewProcessGroup | createNoWindow,
	}
}
