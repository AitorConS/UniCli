package vm

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// --- state machine tests ---

func TestVM_transition_valid(t *testing.T) {
	cases := []struct {
		name string
		from State
		to   State
	}{
		{"created→starting", StateCreated, StateStarting},
		{"starting→running", StateStarting, StateRunning},
		{"starting→stopped", StateStarting, StateStopped},
		{"running→stopping", StateRunning, StateStopping},
		{"running→stopped", StateRunning, StateStopped},
		{"stopping→stopped", StateStopping, StateStopped},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			v := &VM{ID: "test", State: tc.from, done: make(chan struct{})}
			require.NoError(t, v.transition(tc.to))
			require.Equal(t, tc.to, v.GetState())
		})
	}
}

func TestVM_transition_invalid(t *testing.T) {
	cases := []struct {
		from State
		to   State
	}{
		{StateCreated, StateRunning},
		{StateCreated, StateStopped},
		{StateRunning, StateCreated},
		{StateStopped, StateRunning},
		{StateStopped, StateStarting},
	}
	for _, tc := range cases {
		v := &VM{ID: "test", State: tc.from, done: make(chan struct{})}
		require.Error(t, v.transition(tc.to))
		require.Equal(t, tc.from, v.GetState())
	}
}

func TestVM_done_closed_on_stopped(t *testing.T) {
	v := &VM{ID: "test", State: StateRunning, done: make(chan struct{})}
	require.NoError(t, v.transition(StateStopped))
	select {
	case <-v.Done():
	default:
		t.Fatal("done channel not closed after StateStopped")
	}
}

// --- Store tests ---

func TestStore_Create(t *testing.T) {
	s := NewStore()
	v, err := s.Create(Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)
	require.NotEmpty(t, v.ID)
	require.Equal(t, StateCreated, v.GetState())
}

func TestStore_Get(t *testing.T) {
	s := NewStore()
	v, err := s.Create(Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)
	got, err := s.Get(v.ID)
	require.NoError(t, err)
	require.Equal(t, v.ID, got.ID)
}

func TestStore_GetNotFound(t *testing.T) {
	s := NewStore()
	_, err := s.Get("nonexistent")
	require.Error(t, err)
}

func TestStore_List(t *testing.T) {
	s := NewStore()
	_, err := s.Create(Config{ImagePath: "a.img", Memory: "256M"})
	require.NoError(t, err)
	_, err = s.Create(Config{ImagePath: "b.img", Memory: "256M"})
	require.NoError(t, err)
	require.Len(t, s.List(), 2)
}

func TestStore_Remove(t *testing.T) {
	s := NewStore()
	v, err := s.Create(Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)
	require.NoError(t, s.Remove(v.ID))
	require.Empty(t, s.List())
}

func TestStore_RemoveNotFound(t *testing.T) {
	s := NewStore()
	require.Error(t, s.Remove("nonexistent"))
}

// --- QEMUManager tests (with injected fake command) ---

func fakeManager(cmdName string, args ...string) *QEMUManager {
	return NewQEMUManager("fake-qemu", WithCommandFunc(func(_ string, _ ...string) *exec.Cmd {
		return exec.Command(cmdName, args...)
	}))
}

func TestQEMUManager_Create(t *testing.T) {
	mgr := fakeManager("true")
	v, err := mgr.Create(context.Background(), Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)
	require.NotEmpty(t, v.ID)
	require.Equal(t, StateCreated, v.GetState())
}

func TestQEMUManager_Remove_not_stopped(t *testing.T) {
	mgr := fakeManager("true")
	v, err := mgr.Create(context.Background(), Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)
	require.Error(t, mgr.Remove(context.Background(), v.ID))
}

func TestQEMUManager_Start_transitions_to_running(t *testing.T) {
	mgr := fakeManager("sleep", "30")
	v, err := mgr.Create(context.Background(), Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)

	require.NoError(t, mgr.Start(context.Background(), v.ID))
	require.Equal(t, StateRunning, v.GetState())

	require.NoError(t, mgr.Stop(context.Background(), v.ID))
	select {
	case <-v.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("VM did not stop")
	}
}

func TestQEMUManager_Stop_transitions_to_stopped(t *testing.T) {
	mgr := fakeManager("sleep", "30")
	v, err := mgr.Create(context.Background(), Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)

	require.NoError(t, mgr.Start(context.Background(), v.ID))
	require.NoError(t, mgr.Stop(context.Background(), v.ID))

	select {
	case <-v.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("VM did not stop after kill")
	}
	require.Equal(t, StateStopped, v.GetState())
}

func TestQEMUManager_process_exit_stops_vm(t *testing.T) {
	mgr := fakeManager("true") // exits immediately
	v, err := mgr.Create(context.Background(), Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)

	require.NoError(t, mgr.Start(context.Background(), v.ID))

	select {
	case <-v.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("VM did not reach StateStopped after process exit")
	}
	require.Equal(t, StateStopped, v.GetState())
}

func TestQEMUManager_Remove_after_stop(t *testing.T) {
	mgr := fakeManager("true")
	v, err := mgr.Create(context.Background(), Config{ImagePath: "test.img", Memory: "256M"})
	require.NoError(t, err)

	require.NoError(t, mgr.Start(context.Background(), v.ID))
	<-v.Done()

	require.NoError(t, mgr.Remove(context.Background(), v.ID))
	require.Empty(t, mgr.List())
}

func TestQEMUManager_List(t *testing.T) {
	mgr := fakeManager("sleep", "30")
	_, err := mgr.Create(context.Background(), Config{ImagePath: "a.img", Memory: "256M"})
	require.NoError(t, err)
	_, err = mgr.Create(context.Background(), Config{ImagePath: "b.img", Memory: "256M"})
	require.NoError(t, err)
	require.Len(t, mgr.List(), 2)
}
