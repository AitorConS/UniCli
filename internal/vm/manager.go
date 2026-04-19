package vm

import "context"

// Manager manages the lifecycle of unikernel VMs.
type Manager interface {
	// Create registers a new VM with the given config.
	Create(ctx context.Context, cfg Config) (*VM, error)
	// Start launches the QEMU process for the VM with the given id.
	Start(ctx context.Context, id string) error
	// Stop sends a kill signal to the VM's QEMU process.
	Stop(ctx context.Context, id string) error
	// Remove deletes a stopped VM from the registry.
	Remove(ctx context.Context, id string) error
	// Get returns the VM with the given id.
	Get(id string) (*VM, error)
	// List returns all registered VMs.
	List() []*VM
}
