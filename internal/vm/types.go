package vm

import (
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"
)

// State represents a VM lifecycle state.
type State string

const (
	// StateCreated is the initial state after registration.
	StateCreated State = "created"
	// StateStarting means the QEMU process is being launched.
	StateStarting State = "starting"
	// StateRunning means the QEMU process is alive.
	StateRunning State = "running"
	// StateStopping means a kill signal has been sent.
	StateStopping State = "stopping"
	// StateStopped means the QEMU process has exited.
	StateStopped State = "stopped"
)

// validTransitions defines the allowed state machine edges.
var validTransitions = map[State][]State{
	StateCreated:  {StateStarting},
	StateStarting: {StateRunning, StateStopped},
	StateRunning:  {StateStopping, StateStopped},
	StateStopping: {StateStopped},
	StateStopped:  {},
}

// Config holds the parameters used to create a VM.
type Config struct {
	// ImagePath is the raw disk image containing the kernel and application.
	ImagePath string
	// Memory is the QEMU memory string (e.g. "256M").
	Memory string
	// CPUs is the number of virtual CPUs; 0 uses QEMU default.
	CPUs int
	// NetworkName is the TAP interface name to attach; empty disables networking.
	NetworkName string
}

// process abstracts an OS process for testability.
type process interface {
	kill() error
}

// VM is a managed unikernel instance. All exported fields are read-only after
// Start; internal mutation is guarded by mu.
type VM struct {
	// ID uniquely identifies the VM.
	ID string
	// Cfg is the configuration the VM was created with.
	Cfg Config
	// State is the current lifecycle state.
	State State
	// CreatedAt is when the VM was registered.
	CreatedAt time.Time
	// StartedAt is when the QEMU process started (nil until then).
	StartedAt *time.Time
	// StoppedAt is when the QEMU process exited (nil until then).
	StoppedAt *time.Time

	mu   sync.RWMutex
	proc process
	done chan struct{}
}

// Done returns a channel that is closed when the VM reaches StateStopped.
func (v *VM) Done() <-chan struct{} {
	return v.done
}

// GetState returns the current state under a read lock.
func (v *VM) GetState() State {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.State
}

// transition atomically moves v to state to, validating the edge and logging.
func (v *VM) transition(to State) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !slices.Contains(validTransitions[v.State], to) {
		return fmt.Errorf("invalid transition %s → %s", v.State, to)
	}
	from := v.State
	v.State = to
	slog.Info("vm state transition", "vm_id", v.ID, "from", from, "to", to)
	if to == StateStopped {
		close(v.done)
	}
	return nil
}
