//go:build linux

package vm

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// guestExitCmd returns a fake hypervisor process that terminates the way QEMU
// does with `-device isa-debug-exit` when the guest calls exit(guestCode): the
// process status becomes (guestCode<<1)|1. It drives the monitor's on-failure
// decision in tests without a real VM. guestExitCmd(0) is a clean shutdown
// (status 1); guestExitCmd(3) is a crash (status 7).
func guestExitCmd(guestCode int) *exec.Cmd {
	status := ((guestCode << 1) | 1) & 0xFF
	return exec.Command("sh", "-c", fmt.Sprintf("exit %d", status))
}

// waitStatus runs a process that exits with the given status and returns the
// error from Wait (nil for status 0, *exec.ExitError otherwise).
func waitStatus(t *testing.T, status int) error {
	t.Helper()
	return exec.Command("sh", "-c", fmt.Sprintf("exit %d", status&0xFF)).Run()
}

// ---------------------------------------------------------------------------
// BUG-010: `--restart on-failure` never fires — the guest exit code was lost.
// ---------------------------------------------------------------------------

func TestBuildCmd_IsaDebugExit(t *testing.T) {
	// QEMU must be launched with isa-debug-exit; without it a guest exit(N) just
	// powers the VM off and QEMU exits 0, so a crash is indistinguishable from a
	// clean shutdown and on-failure can never fire.
	mgr := NewQEMUManager("fake-qemu")
	args := captureArgs(mgr, Config{ImagePath: "disk.img", Memory: "256M"})
	found := false
	for i, a := range args {
		if a == "-device" && i+1 < len(args) && strings.Contains(args[i+1], "isa-debug-exit") {
			found = true
		}
	}
	require.True(t, found, "qemu must be launched with isa-debug-exit to observe guest exit codes")
}

func TestGuestExitCode_Decoding(t *testing.T) {
	cases := []struct {
		name     string
		status   int
		wantCode int
		wantOK   bool
	}{
		{"clean guest exit(0) -> status 1", 1, 0, true},
		{"crash guest exit(1) -> status 3", 3, 1, true},
		{"crash guest exit(3) -> status 7", 7, 3, true},
		{"even status is not an isa-debug-exit code", 2, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, ok := guestExitCode(waitStatus(t, tc.status))
			require.Equal(t, tc.wantOK, ok)
			if tc.wantOK {
				require.Equal(t, tc.wantCode, code)
			}
		})
	}
	// A clean QEMU exit (status 0) carries no guest code.
	_, ok := guestExitCode(nil)
	require.False(t, ok)
}

func TestGuestExitCode_Signaled(t *testing.T) {
	cmd := exec.Command("sleep", "10")
	require.NoError(t, cmd.Start())
	require.NoError(t, cmd.Process.Kill())
	_, ok := guestExitCode(cmd.Wait())
	require.False(t, ok, "a signaled process carries no guest exit code")
}

func TestIsFailureExit(t *testing.T) {
	require.False(t, isFailureExit(nil), "qemu exit 0 (no debug-exit write) is treated as clean")
	require.False(t, isFailureExit(waitStatus(t, 1)), "guest exit(0) is a clean shutdown")
	require.True(t, isFailureExit(waitStatus(t, 7)), "guest exit(3) is a crash")
	require.True(t, isFailureExit(waitStatus(t, 3)), "guest exit(1) is a crash")
}

func TestMonitor_RestartOnFailure_CleanExit_NoRestart(t *testing.T) {
	// A clean guest exit(0) surfaces as process status 1. Before isa-debug-exit
	// this was indistinguishable from a crash; on-failure must NOT restart it.
	cmdFunc := func(_ context.Context, _ string, _ ...string) *exec.Cmd {
		return guestExitCmd(0)
	}
	mgr := NewQEMUManager("fake-qemu", WithCommandFunc(cmdFunc))
	v, err := mgr.Create(context.Background(), Config{
		ImagePath: "test.img",
		Memory:    "256M",
		Restart:   RestartConfig{Policy: RestartOnFailure},
	})
	require.NoError(t, err)
	require.NoError(t, mgr.Start(context.Background(), v.ID))
	select {
	case <-v.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("vm did not exit")
	}
	require.Never(t, func() bool {
		for _, vm := range mgr.List() {
			if vm.GetRestartCount() >= 1 {
				return true
			}
		}
		return false
	}, 2*time.Second, 200*time.Millisecond)
}

// ---------------------------------------------------------------------------
// BUG-008: --health-check dialed the host-published port, not the guest service.
// ---------------------------------------------------------------------------

func TestProbeTarget_UsesGuestServiceWhenIPPresent(t *testing.T) {
	// -p 18080:8080 with --health-check tcp:8080: the probe must reach the GUEST
	// service (guestIP:8080), not 127.0.0.1:8080 (the host side, where nothing of
	// this VM listens — the old behavior marked healthy VMs unhealthy).
	v := &VM{Cfg: Config{
		IPAddress: "10.100.0.5",
		PortMaps:  []PortMap{{HostPort: 18080, GuestPort: 8080, Protocol: ProtocolTCP}},
	}}
	require.Equal(t, "10.100.0.5:8080", probeTarget(v, &HealthCheckConfig{Type: "tcp", Port: 8080}))
	require.Equal(t, "http://10.100.0.5:8080/health",
		probeTarget(v, &HealthCheckConfig{Type: "http", Port: 8080, Path: "/health"}))
}

func TestProbeTarget_DefaultsToGuestPortOfFirstMap(t *testing.T) {
	v := &VM{Cfg: Config{IPAddress: "10.0.0.2", PortMaps: []PortMap{{HostPort: 9000, GuestPort: 80}}}}
	require.Equal(t, "10.0.0.2:80", probeTarget(v, &HealthCheckConfig{Type: "tcp"}))
}

func TestProbeTarget_CrossVMCollisionProbesOwnGuest(t *testing.T) {
	// Two VMs collide on the same host port but each serves on its own guest IP.
	// Each probe must target its OWN guest, so a host-port collision cannot make
	// one VM's health reflect the other's.
	a := &VM{Cfg: Config{IPAddress: "10.0.0.2", PortMaps: []PortMap{{HostPort: 19090, GuestPort: 8080}}}}
	b := &VM{Cfg: Config{IPAddress: "10.0.0.3", PortMaps: []PortMap{{HostPort: 19090, GuestPort: 8080}}}}
	cfg := &HealthCheckConfig{Type: "tcp", Port: 8080}
	require.Equal(t, "10.0.0.2:8080", probeTarget(a, cfg))
	require.Equal(t, "10.0.0.3:8080", probeTarget(b, cfg))
}

// ---------------------------------------------------------------------------
// BUG-009: --memory-max / --cpu-shares were silently ignored on cgroup failure.
// ---------------------------------------------------------------------------

func TestStart_ResourceLimitFailure_FailsRun(t *testing.T) {
	mgr := fakeManager(true)
	mgr.applyLimits = func(_ *VM, _ int) error { return fmt.Errorf("permission denied") }
	v, err := mgr.Create(context.Background(), Config{
		ImagePath: "test.img",
		Memory:    "256M",
		MemoryMax: 128 << 20,
	})
	require.NoError(t, err)
	err = mgr.Start(context.Background(), v.ID)
	require.Error(t, err, "an unappliable resource limit must fail the run, not be silently ignored")
	require.Contains(t, err.Error(), "permission denied")
	select {
	case <-v.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("aborted VM did not reach stopped")
	}
	require.Equal(t, StateStopped, v.GetState())
}

func TestStart_ResourceLimitApplied_Runs(t *testing.T) {
	mgr := fakeManager(true)
	var appliedPid int
	mgr.applyLimits = func(_ *VM, pid int) error { appliedPid = pid; return nil }
	v, err := mgr.Create(context.Background(), Config{
		ImagePath: "test.img",
		Memory:    "256M",
		CPUShares: 512,
	})
	require.NoError(t, err)
	require.NoError(t, mgr.Start(context.Background(), v.ID))
	require.Positive(t, appliedPid, "limits must be applied to the running hypervisor pid")
	require.Equal(t, StateRunning, v.GetState())
	require.NoError(t, mgr.Stop(context.Background(), v.ID))
	<-v.Done()
}

// ---------------------------------------------------------------------------
// BUG-005: a port-forwarder bind failure was swallowed as a WARN.
// ---------------------------------------------------------------------------

func TestStart_PortForwarderBindFailure_FailsRun(t *testing.T) {
	// Occupy a host port so the forwarder cannot bind it.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer func() { _ = ln.Close() }()
	hostPort := ln.Addr().(*net.TCPAddr).Port

	mgr := fakeManager(true)
	v, err := mgr.Create(context.Background(), Config{
		ImagePath:   "test.img",
		Memory:      "256M",
		NetworkName: "testnet",
		IPAddress:   "10.0.0.2",
		BridgeName:  "jerboa-br0",
		PortMaps: []PortMap{{
			HostPort: uint16(hostPort), GuestPort: 8080,
			Protocol: ProtocolTCP, BindAddr: "127.0.0.1",
		}},
	})
	require.NoError(t, err)
	err = mgr.Start(context.Background(), v.ID)
	require.Error(t, err, "a port-forwarder bind failure must fail the run, not leave a 'running' VM with a dead port")
	require.Contains(t, err.Error(), "address already in use")
	select {
	case <-v.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("aborted VM did not reach stopped")
	}
	require.Equal(t, StateStopped, v.GetState())
}

// ---------------------------------------------------------------------------
// BUG-002: volumes that fail to mount lost data silently.
// ---------------------------------------------------------------------------

func TestDetectMountFailure(t *testing.T) {
	cases := []struct {
		log      string
		wantMP   string
		wantFail bool
	}{
		{"[0.019557] storage: mount point /data not found\n", "/data", true},
		{"storage: invalid mount point /var/lib/pg\n", "/var/lib/pg", true},
		{"[0.02] mounting volume at /data\nboot ok\n", "", false},
		{"", "", false},
	}
	for _, tc := range cases {
		mp, failed := detectMountFailure([]byte(tc.log))
		require.Equal(t, tc.wantFail, failed, "log=%q", tc.log)
		require.Equal(t, tc.wantMP, mp, "log=%q", tc.log)
	}
}

func TestWatchVolumeMounts_SurfacesFailureAsWarning(t *testing.T) {
	old := mountCheckInterval
	mountCheckInterval = 10 * time.Millisecond
	defer func() { mountCheckInterval = old }()

	v := &VM{ID: "mnt", done: make(chan struct{})}
	_, _ = v.logBuf.Write([]byte("[0.02] storage: mount point /data not found\n"))

	done := make(chan struct{})
	go func() { watchVolumeMounts(context.Background(), v); close(done) }()

	select {
	case <-done: // watcher returns after recording the warning
	case <-time.After(2 * time.Second):
		t.Fatal("watcher did not detect the mount failure")
	}
	warnings := v.Warnings()
	require.Len(t, warnings, 1)
	require.Contains(t, warnings[0], "/data")
}

func TestWatchVolumeMounts_NoFailure_NoWarning(t *testing.T) {
	old := mountCheckInterval
	mountCheckInterval = 5 * time.Millisecond
	defer func() { mountCheckInterval = old }()

	v := &VM{ID: "ok", done: make(chan struct{})}
	_, _ = v.logBuf.Write([]byte("[0.02] mounting volume at /data\nvoltest boot\n"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { watchVolumeMounts(ctx, v); close(done) }()
	// Let it poll a few times, then stop it and confirm no warning was recorded.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done
	require.Empty(t, v.Warnings(), "a VM whose volumes mounted must not be warned")
}
