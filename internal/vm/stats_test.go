//go:build linux

package vm

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestVM_Stats_Fallback(t *testing.T) {
	v := &VM{
		ID:        "test-vm",
		State:     StateRunning,
		Cfg:       Config{ImagePath: "test.img", Memory: "256M"},
		CreatedAt: time.Now(),
		done:      make(chan struct{}),
	}

	stats := v.Stats()
	require.Equal(t, "test-vm", stats.ID)
	require.Equal(t, "running", stats.State)
	require.Equal(t, "fallback", stats.Source)
	require.WithinDuration(t, time.Now(), stats.Timestamp, 2*time.Second)
}

func TestVM_Stats_WithProvider(t *testing.T) {
	v := &VM{
		ID:        "test-vm",
		State:     StateRunning,
		Cfg:       Config{ImagePath: "test.img", Memory: "256M"},
		CreatedAt: time.Now(),
		done:      make(chan struct{}),
	}

	expected := RuntimeStats{
		ID:         "test-vm",
		State:      "running",
		CPUPct:     42.5,
		MemBytes:   1048576,
		NetRxBytes: 500,
		NetTxBytes: 800,
		Timestamp:  time.Now(),
		Source:     "procfs",
	}

	v.SetStatsProvider(func() RuntimeStats {
		return expected
	})

	stats := v.Stats()
	require.InDelta(t, expected.CPUPct, stats.CPUPct, 1e-9)
	require.Equal(t, expected.MemBytes, stats.MemBytes)
	require.Equal(t, expected.NetRxBytes, stats.NetRxBytes)
	require.Equal(t, expected.NetTxBytes, stats.NetTxBytes)
	require.Equal(t, "procfs", stats.Source)
}

func TestProcStatsCollector_FallbackOnNonLinux(t *testing.T) {
	collector := NoopStatsCollector{ID: "vm1", State: "stopped"}
	stats := collector.Collect()
	require.Equal(t, "vm1", stats.ID)
	require.Equal(t, "stopped", stats.State)
	require.Equal(t, "fallback", stats.Source)
	require.InDelta(t, 0.0, stats.CPUPct, 1e-9)
	require.Equal(t, int64(0), stats.MemBytes)
}

// TestReadTapStats_PerVMAndGuestCentric is the F-007 regression: network stats
// must come from the VM's own tap device (not the shared distro eth0), and be
// reported from the guest's point of view — the host-side tap rx/tx are flipped.
func TestReadTapStats_PerVMAndGuestCentric(t *testing.T) {
	root := t.TempDir()
	writeTap := func(tap string, rx, tx int64) {
		dir := filepath.Join(root, tap, "statistics")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "rx_bytes"), []byte(strconv.FormatInt(rx, 10)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "tx_bytes"), []byte(strconv.FormatInt(tx, 10)+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Two different VMs' taps carry different traffic — the old code returned the
	// same distro eth0 number for both.
	writeTap("jerboa-tap-aaa", 100, 900) // host rx=100, tx=900
	writeTap("jerboa-tap-bbb", 5, 7)

	old := sysClassNet
	sysClassNet = root
	defer func() { sysClassNet = old }()

	rx, tx, err := readTapStats("jerboa-tap-aaa")
	if err != nil {
		t.Fatal(err)
	}
	// guest RX = host tx (900), guest TX = host rx (100).
	if rx != 900 || tx != 100 {
		t.Fatalf("tap aaa: got guestRx=%d guestTx=%d, want 900/100", rx, tx)
	}

	rx2, tx2, err := readTapStats("jerboa-tap-bbb")
	if err != nil {
		t.Fatal(err)
	}
	if rx2 != 7 || tx2 != 5 {
		t.Fatalf("tap bbb: got guestRx=%d guestTx=%d, want 7/5", rx2, tx2)
	}
	if rx == rx2 {
		t.Fatal("distinct VMs must report distinct network counters (F-007)")
	}

	// A missing tap yields an error (caller falls back to zero), never a stale value.
	if _, _, err := readTapStats("nope"); err == nil {
		t.Fatal("missing tap should error")
	}
}
