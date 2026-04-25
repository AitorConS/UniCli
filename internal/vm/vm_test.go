package vm

import (
	"context"
	"os/exec"
	"strings"
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

// --- buildCmd / buildNetArgs / buildEnvArgs tests ---

func captureArgs(mgr *QEMUManager, cfg Config) []string {
	var got []string
	mgr.mkCmd = func(_ string, args ...string) *exec.Cmd {
		got = args
		return exec.Command("true")
	}
	_ = mgr.buildCmd(cfg)
	return got
}

func TestBuildCmd_no_network(t *testing.T) {
	mgr := NewQEMUManager("fake-qemu")
	args := captureArgs(mgr, Config{ImagePath: "disk.img", Memory: "256M"})
	require.Contains(t, args, "-net")
	idx := indexOf(args, "-net")
	require.Equal(t, "none", args[idx+1])
}

func TestBuildCmd_slirp_single_port(t *testing.T) {
	mgr := NewQEMUManager("fake-qemu")
	args := captureArgs(mgr, Config{
		ImagePath: "disk.img",
		Memory:    "256M",
		PortMaps:  []PortMap{{HostPort: 8080, GuestPort: 80, Protocol: ProtocolTCP}},
	})
	// -net none must NOT appear
	require.NotContains(t, args, "none")
	// -netdev user,id=net0,hostfwd=tcp::8080-:80
	idx := indexOf(args, "-netdev")
	require.GreaterOrEqual(t, idx, 0)
	require.Contains(t, args[idx+1], "hostfwd=tcp::8080-:80")
}

func TestBuildCmd_slirp_multiple_ports(t *testing.T) {
	mgr := NewQEMUManager("fake-qemu")
	args := captureArgs(mgr, Config{
		ImagePath: "disk.img",
		Memory:    "256M",
		PortMaps: []PortMap{
			{HostPort: 8080, GuestPort: 80, Protocol: ProtocolTCP},
			{HostPort: 5353, GuestPort: 53, Protocol: ProtocolUDP},
		},
	})
	idx := indexOf(args, "-netdev")
	require.GreaterOrEqual(t, idx, 0)
	netdev := args[idx+1]
	require.Contains(t, netdev, "hostfwd=tcp::8080-:80")
	require.Contains(t, netdev, "hostfwd=udp::5353-:53")
}

func TestBuildCmd_tap_overrides_portmaps(t *testing.T) {
	mgr := NewQEMUManager("fake-qemu")
	args := captureArgs(mgr, Config{
		ImagePath:   "disk.img",
		Memory:      "256M",
		NetworkName: "uni-tap0",
		PortMaps:    []PortMap{{HostPort: 8080, GuestPort: 80, Protocol: ProtocolTCP}},
	})
	idx := indexOf(args, "-netdev")
	require.GreaterOrEqual(t, idx, 0)
	require.Contains(t, args[idx+1], "tap,id=net0,ifname=uni-tap0")
	require.NotContains(t, args[idx+1], "hostfwd")
}

func TestBuildCmd_env_vars(t *testing.T) {
	mgr := NewQEMUManager("fake-qemu")
	args := captureArgs(mgr, Config{
		ImagePath: "disk.img",
		Memory:    "256M",
		Env:       []string{"FOO=bar", "PORT=8080"},
	})
	idx := indexOf(args, "-fw_cfg")
	require.GreaterOrEqual(t, idx, 0, "-fw_cfg flag must be present")
	fwcfg := args[idx+1]
	require.True(t, strings.HasPrefix(fwcfg, "name=opt/uni/env,string="))
	require.Contains(t, fwcfg, "FOO=bar")
	require.Contains(t, fwcfg, "PORT=8080")
}

func TestBuildCmd_no_env_no_fwcfg(t *testing.T) {
	mgr := NewQEMUManager("fake-qemu")
	args := captureArgs(mgr, Config{ImagePath: "disk.img", Memory: "256M"})
	require.NotContains(t, args, "-fw_cfg")
}

func TestBuildCmd_cpus(t *testing.T) {
	mgr := NewQEMUManager("fake-qemu")
	args := captureArgs(mgr, Config{ImagePath: "disk.img", Memory: "512M", CPUs: 4})
	idx := indexOf(args, "-smp")
	require.GreaterOrEqual(t, idx, 0)
	require.Equal(t, "4", args[idx+1])
}

func indexOf(slice []string, s string) int {
	for i, v := range slice {
		if v == s {
			return i
		}
	}
	return -1
}
