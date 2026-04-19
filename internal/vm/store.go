package vm

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"
)

// Store is a thread-safe in-memory registry of VMs.
type Store struct {
	mu  sync.RWMutex
	vms map[string]*VM
}

// NewStore returns an empty Store.
func NewStore() *Store {
	return &Store{vms: make(map[string]*VM)}
}

// Create registers a new VM with the given config and returns it.
func (s *Store) Create(cfg Config) (*VM, error) {
	id, err := newID()
	if err != nil {
		return nil, fmt.Errorf("create vm: generate id: %w", err)
	}
	v := &VM{
		ID:        id,
		Cfg:       cfg,
		State:     StateCreated,
		CreatedAt: time.Now(),
		done:      make(chan struct{}),
	}
	s.mu.Lock()
	s.vms[id] = v
	s.mu.Unlock()
	return v, nil
}

// Get returns the VM with the given id or an error if not found.
func (s *Store) Get(id string) (*VM, error) {
	s.mu.RLock()
	v, ok := s.vms[id]
	s.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("vm %s not found", id)
	}
	return v, nil
}

// List returns a snapshot of all VMs in the store.
func (s *Store) List() []*VM {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*VM, 0, len(s.vms))
	for _, v := range s.vms {
		out = append(out, v)
	}
	return out
}

// Remove deletes the VM with the given id. Returns an error if not found.
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.vms[id]; !ok {
		return fmt.Errorf("vm %s not found", id)
	}
	delete(s.vms, id)
	return nil
}

func newID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand read: %w", err)
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:]), nil
}
