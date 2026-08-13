//go:build linux

package vm

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupBase = "/sys/fs/cgroup"

type CgroupLimit struct {
	CPUShares uint64
	MemoryMax int64
}

type CgroupManager struct {
	vmID string
	path string
}

func NewCgroupManager(vmID string) *CgroupManager {
	return &CgroupManager{
		vmID: vmID,
		path: filepath.Join(cgroupBase, "jerboa", vmID),
	}
}

func (m *CgroupManager) Apply(pid int, limits CgroupLimit) error {
	if err := os.MkdirAll(m.path, 0o755); err != nil {
		return fmt.Errorf("cgroup mkdir %s: %w", m.path, err)
	}
	if limits.CPUShares > 0 {
		if err := os.WriteFile(filepath.Join(m.path, "cpu.weight"), []byte(strconv.FormatUint(limits.CPUShares, 10)), 0o644); err != nil {
			return fmt.Errorf("cgroup set cpu.weight: %w", err)
		}
		slog.Info("cgroup: set cpu weight", "vm_id", m.vmID, "weight", limits.CPUShares)
	}
	if limits.MemoryMax > 0 {
		if err := os.WriteFile(filepath.Join(m.path, "memory.max"), []byte(strconv.FormatInt(limits.MemoryMax, 10)), 0o644); err != nil {
			return fmt.Errorf("cgroup set memory.max: %w", err)
		}
		slog.Info("cgroup: set memory.max", "vm_id", m.vmID, "bytes", limits.MemoryMax)
	}
	if err := os.WriteFile(filepath.Join(m.path, "cgroup.procs"), []byte(strconv.Itoa(pid)), 0o644); err != nil {
		return fmt.Errorf("cgroup move pid %d: %w", pid, err)
	}
	slog.Info("cgroup: moved pid", "vm_id", m.vmID, "pid", pid)
	return nil
}

func (m *CgroupManager) Remove() error {
	procs, err := os.ReadFile(filepath.Join(m.path, "cgroup.procs"))
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cgroup read procs: %w", err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(procs)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		rootProcs := filepath.Join(cgroupBase, "cgroup.procs")
		if err := os.WriteFile(rootProcs, []byte(line), 0o644); err != nil {
			slog.Warn("cgroup: failed to move pid to root", "vm_id", m.vmID, "pid", line, "err", err)
		}
	}
	if err := os.RemoveAll(m.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cgroup rmdir %s: %w", m.path, err)
	}
	slog.Info("cgroup: removed", "vm_id", m.vmID)
	return nil
}

func IsCgroupV2Available() bool {
	_, err := os.Stat(filepath.Join(cgroupBase, "cgroup.controllers"))
	return err == nil
}

// defaultApplyLimits places the hypervisor process (pid) into a per-VM cgroup v2
// subtree carrying the requested CPU weight / memory cap, and records the
// manager on v so the limits are torn down when the VM stops.
//
// It returns an error — rather than logging and continuing — when the limit
// cannot be applied. Callers surface that as a failed Start: a user who asked
// for isolation must not silently get a VM with none (which can let one guest
// starve the host or its neighbours). The error text points at the usual WSL2
// cause (cgroup v2 delegation not set up for the daemon).
func defaultApplyLimits(v *VM, pid int) error {
	if !IsCgroupV2Available() {
		return fmt.Errorf("resource limits (--cpu-shares/--memory-max) require cgroup v2, which is not available on this host")
	}
	cg := NewCgroupManager(v.ID)
	if err := cg.Apply(pid, CgroupLimit{CPUShares: v.Cfg.CPUShares, MemoryMax: v.Cfg.MemoryMax}); err != nil {
		return fmt.Errorf("apply resource limits: %w (on WSL2 this usually means cgroup v2 delegation is not set up for the daemon)", err)
	}
	v.mu.Lock()
	v.cgroupMgr = cg
	v.mu.Unlock()
	return nil
}
