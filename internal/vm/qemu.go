package vm

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// CommandFunc builds an exec.Cmd. Defaults to exec.Command; replaceable in tests.
type CommandFunc func(name string, args ...string) *exec.Cmd

// Option configures a QEMUManager.
type Option func(*QEMUManager)

// WithCommandFunc injects a custom command builder (for tests).
func WithCommandFunc(fn CommandFunc) Option {
	return func(m *QEMUManager) { m.mkCmd = fn }
}

// QEMUManager implements Manager by spawning qemu-system-x86_64 processes.
type QEMUManager struct {
	store   *Store
	qemuBin string
	mkCmd   CommandFunc
}

// NewQEMUManager returns a QEMUManager using qemuBin as the QEMU executable.
func NewQEMUManager(qemuBin string, opts ...Option) *QEMUManager {
	m := &QEMUManager{
		store:   NewStore(),
		qemuBin: qemuBin,
		mkCmd:   exec.Command,
	}
	for _, o := range opts {
		o(m)
	}
	return m
}

// Create registers a new VM with the given config.
func (m *QEMUManager) Create(_ context.Context, cfg Config) (*VM, error) {
	v, err := m.store.Create(cfg)
	if err != nil {
		return nil, fmt.Errorf("qemu manager create: %w", err)
	}
	return v, nil
}

// Start launches the QEMU process for the VM identified by id.
func (m *QEMUManager) Start(_ context.Context, id string) error {
	v, err := m.store.Get(id)
	if err != nil {
		return fmt.Errorf("qemu start %s: %w", id, err)
	}
	if err := v.transition(StateStarting); err != nil {
		return fmt.Errorf("qemu start %s: %w", id, err)
	}
	cmd := m.buildCmd(v.Cfg)
	if err := cmd.Start(); err != nil {
		if tErr := v.transition(StateStopped); tErr != nil {
			return fmt.Errorf("qemu start %s: launch: %w; also failed to stop: %v", id, err, tErr)
		}
		return fmt.Errorf("qemu start %s: launch: %w", id, err)
	}
	now := time.Now()
	v.mu.Lock()
	v.proc = &osProcess{cmd.Process}
	v.StartedAt = &now
	v.mu.Unlock()
	if err := v.transition(StateRunning); err != nil {
		_ = cmd.Process.Kill()
		return fmt.Errorf("qemu start %s: %w", id, err)
	}
	go m.monitor(v, cmd)
	return nil
}

// Stop kills the QEMU process for the VM identified by id.
func (m *QEMUManager) Stop(_ context.Context, id string) error {
	v, err := m.store.Get(id)
	if err != nil {
		return fmt.Errorf("qemu stop %s: %w", id, err)
	}
	if err := v.transition(StateStopping); err != nil {
		return fmt.Errorf("qemu stop %s: %w", id, err)
	}
	v.mu.RLock()
	proc := v.proc
	v.mu.RUnlock()
	if proc != nil {
		if err := proc.kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
			return fmt.Errorf("qemu stop %s: kill: %w", id, err)
		}
	}
	return nil
}

// Remove deletes a stopped VM from the registry.
func (m *QEMUManager) Remove(_ context.Context, id string) error {
	v, err := m.store.Get(id)
	if err != nil {
		return fmt.Errorf("qemu remove %s: %w", id, err)
	}
	if st := v.GetState(); st != StateStopped {
		return fmt.Errorf("qemu remove %s: vm is %s, must be stopped first", id, st)
	}
	if err := m.store.Remove(id); err != nil {
		return fmt.Errorf("qemu remove %s: %w", id, err)
	}
	return nil
}

// Get returns the VM with the given id.
func (m *QEMUManager) Get(id string) (*VM, error) {
	v, err := m.store.Get(id)
	if err != nil {
		return nil, fmt.Errorf("qemu get %s: %w", id, err)
	}
	return v, nil
}

// List returns all registered VMs.
func (m *QEMUManager) List() []*VM {
	return m.store.List()
}

func (m *QEMUManager) buildCmd(cfg Config) *exec.Cmd {
	args := []string{
		"-m", cfg.Memory,
		"-drive", "file=" + cfg.ImagePath + ",format=raw,if=virtio",
		"-nographic",
		"-serial", "stdio",
		"-no-reboot",
	}
	if cfg.CPUs > 0 {
		args = append(args, "-smp", fmt.Sprintf("%d", cfg.CPUs))
	}
	if cfg.NetworkName != "" {
		args = append(args,
			"-netdev", "tap,id=net0,ifname="+cfg.NetworkName,
			"-device", "virtio-net-pci,netdev=net0",
		)
	} else {
		args = append(args, "-net", "none")
	}
	return m.mkCmd(m.qemuBin, args...)
}

func (m *QEMUManager) monitor(v *VM, cmd *exec.Cmd) {
	_ = cmd.Wait()
	now := time.Now()
	v.mu.Lock()
	v.StoppedAt = &now
	v.mu.Unlock()
	if err := v.transition(StateStopped); err != nil {
		// Stop() may have already transitioned to StateStopping/Stopped.
		slog.Debug("monitor: transition to stopped", "vm_id", v.ID, "err", err)
	}
}

// osProcess wraps *os.Process to implement the package-private process interface.
type osProcess struct{ p *os.Process }

func (o *osProcess) kill() error {
	if err := o.p.Kill(); err != nil {
		return fmt.Errorf("kill process %d: %w", o.p.Pid, err)
	}
	return nil
}
