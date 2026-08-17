//go:build linux

package vm

import (
	"errors"
	"os/exec"
)

// guestExitCode extracts the unikernel guest's own exit code from the hypervisor
// process's wait result.
//
// QEMU is launched with `-device isa-debug-exit,iobase=0x501,iosize=1`. Nanos
// writes the process exit status to port 0x501 on shutdown (QEMU_HALT), so QEMU
// terminates with process status (guestCode<<1)|1 — always odd. Reversing that
// encoding recovers the guest code: a clean guest exit(0) yields process status
// 1 → code 0; a crash exit(3) yields status 7 → code 3.
//
// ok is false when the status does not carry a guest code: the process was
// signaled (e.g. SIGKILL), exited 0 (no debug-exit write — a Firecracker guest,
// which has no such device, or an externally reset VM), or exited with an even
// status (QEMU's own failure, not a guest write). Callers fall back to the raw
// hypervisor result in those cases.
func guestExitCode(waitErr error) (code int, ok bool) {
	if waitErr == nil {
		return 0, false
	}
	var ee *exec.ExitError
	if !errors.As(waitErr, &ee) {
		return 0, false
	}
	status := ee.ExitCode()
	if status <= 0 { // -1 == signaled; 0 == clean process exit with no debug-exit write
		return 0, false
	}
	if status&1 == 0 { // even status: not an isa-debug-exit encoding
		return 0, false
	}
	return status >> 1, true
}

// isFailureExit reports whether a VM's exit should be treated as a failure for
// the `on-failure` restart policy.
//
// When the guest exit code is recoverable (QEMU + isa-debug-exit), the decision
// is made on THAT code: a non-zero guest exit is a crash and restarts, a zero
// guest exit is a clean shutdown and does not. This is the whole point of the
// isa-debug-exit wiring — previously a guest crash powered the VM off and left
// the hypervisor exiting 0, so on-failure never fired.
//
// When the guest code cannot be recovered (a signaled process, or a backend
// without a debug-exit channel such as Firecracker), it falls back to the raw
// hypervisor result: any abnormal (non-nil) hypervisor exit is treated as a
// failure. That preserves the previous behavior on those paths.
func isFailureExit(waitErr error) bool {
	if code, ok := guestExitCode(waitErr); ok {
		return code != 0
	}
	return waitErr != nil
}
