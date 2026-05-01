package vm

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
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

// VolumeMount describes a volume attached to a VM.
type VolumeMount struct {
	// DiskPath is the absolute path to the raw disk image on the host.
	DiskPath string
	// GuestPath is the mount point inside the VM (informational; used by kernel).
	GuestPath string
	// ReadOnly marks the volume as read-only.
	ReadOnly bool
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
	// When PortMaps are set and NetworkName is empty, SLIRP user-mode networking
	// is used automatically so no TAP device is required.
	NetworkName string
	// PortMaps is the list of host-to-guest port forwarding rules.
	// Requires SLIRP or TAP networking; mutually exclusive with "-net none".
	PortMaps []PortMap
	// Env is a list of "KEY=VALUE" environment variable pairs injected at
	// boot time via QEMU fw_cfg. The kernel must read opt/uni/env to consume them.
	Env []string
	// Name is a human-readable identifier for the VM. If empty, the UUID is used.
	Name string
	// Volumes is the list of additional disk images to attach to the VM.
	Volumes []VolumeMount
	// Attach when true, creates a pipe for streaming serial console output.
	Attach bool
	// IPAddress is the static IP address to assign to the VM. Requires TAP
	// networking (NetworkName). If empty, no static IP is configured.
	IPAddress string
	// GatewayIP is the gateway IP for the VM's network. Derived from IPAddress
	// when using TAP networking. Used to assign an IP to the bridge interface.
	GatewayIP string
}

// process abstracts an OS process for testability.
type process interface {
	kill() error
	signal(sig os.Signal) error
}

// safeBuffer is a concurrency-safe write-only byte buffer used for VM log capture.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n, err := b.buf.Write(p)
	if err != nil {
		return n, fmt.Errorf("safe buffer write: %w", err)
	}
	return n, nil
}

func (b *safeBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	cp := make([]byte, b.buf.Len())
	copy(cp, b.buf.Bytes())
	return cp
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

	mu            sync.RWMutex
	proc          process
	done          chan struct{}
	logBuf        safeBuffer
	logPipeReader io.Reader
	logPipeWriter *io.PipeWriter
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

// Logs returns a snapshot of captured QEMU serial console output.
func (v *VM) Logs() []byte {
	return v.logBuf.Bytes()
}

// AttachReader returns a reader that streams QEMU serial console output.
// Returns nil if no attach pipe was created (VM not started in attach mode).
func (v *VM) AttachReader() io.Reader {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.logPipeReader
}

// GetTimes returns the start and stop timestamps under a read lock.
func (v *VM) GetTimes() (startedAt, stoppedAt *time.Time) {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.StartedAt, v.StoppedAt
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
