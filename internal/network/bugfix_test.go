//go:build linux

package network

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func mustCIDR(t *testing.T, cidr string) *net.IPNet {
	t.Helper()
	_, n, err := net.ParseCIDR(cidr)
	require.NoError(t, err)
	return n
}

// ---------------------------------------------------------------------------
// BUG-003: a duplicate static --ip was accepted, putting two VMs on one address.
// ---------------------------------------------------------------------------

func TestReserveIP_RejectsDuplicate(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	_, err = s.Create("testnet", "10.100.0.0/24", "bridge")
	require.NoError(t, err)

	// First reservation of a static IP succeeds.
	require.NoError(t, s.ReserveIP("testnet", "10.100.0.50"))

	// A second VM asking for the same static IP must be rejected with the
	// classifiable sentinel, not silently accepted onto a colliding address.
	err = s.ReserveIP("testnet", "10.100.0.50")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrIPAlreadyAllocated)
}

func TestReserveIP_ConflictsWithDynamicAllocation(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	_, err = s.Create("testnet", "10.100.0.0/24", "bridge")
	require.NoError(t, err)

	// Dynamic allocation hands out .2, then a static request for .2 must clash,
	// and a dynamic allocation must skip a previously reserved static address.
	ip, err := s.AllocateIP("testnet") // .2
	require.NoError(t, err)
	require.Equal(t, "10.100.0.2", ip.String())

	require.ErrorIs(t, s.ReserveIP("testnet", "10.100.0.2"), ErrIPAlreadyAllocated)

	require.NoError(t, s.ReserveIP("testnet", "10.100.0.9"))
	// The next dynamic allocation is .3 (not .9, which is now reserved); keep
	// pulling and assert .9 is never re-handed-out.
	for i := 0; i < 10; i++ {
		got, allocErr := s.AllocateIP("testnet")
		require.NoError(t, allocErr)
		require.NotEqual(t, "10.100.0.9", got.String(), "dynamic allocation must skip a reserved static IP")
	}
}

func TestReserveIP_RejectsOutOfSubnet(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	_, err = s.Create("testnet", "10.100.0.0/24", "bridge")
	require.NoError(t, err)

	err = s.ReserveIP("testnet", "10.100.5.50") // outside the /24
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrIPAlreadyAllocated, "out-of-range is a distinct error, not a duplicate")
}

func TestReserveIP_RejectsNetworkAndBroadcast(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	_, err = s.Create("testnet", "10.100.0.0/24", "bridge")
	require.NoError(t, err)

	// The subnet (network) address and the directed broadcast address are inside
	// the CIDR but are not assignable to a host: reserving either must fail, and
	// with a distinct error (not the duplicate sentinel).
	err = s.ReserveIP("testnet", "10.100.0.0")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrIPAlreadyAllocated, "network address is a distinct error, not a duplicate")

	err = s.ReserveIP("testnet", "10.100.0.255")
	require.Error(t, err)
	require.NotErrorIs(t, err, ErrIPAlreadyAllocated, "broadcast address is a distinct error, not a duplicate")

	// A normal host address in the same subnet is still accepted.
	require.NoError(t, s.ReserveIP("testnet", "10.100.0.50"))
}

func TestReserveIP_UnknownNetwork(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	require.Error(t, s.ReserveIP("nope", "10.100.0.2"))
}

// ---------------------------------------------------------------------------
// BUG-007: overlapping subnets between networks were allowed.
// ---------------------------------------------------------------------------

func TestCreate_RejectsExactSubnetOverlap(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	_, err = s.Create("testnet", "10.100.0.0/24", "bridge")
	require.NoError(t, err)

	_, err = s.Create("app", "10.100.0.0/24", "bridge")
	require.Error(t, err)
	require.Contains(t, err.Error(), "overlaps")
	require.Contains(t, err.Error(), "testnet")
}

func TestCreate_RejectsRangeSubnetOverlap(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	// A wide /16 already exists; a /24 wholly inside it must be rejected too, not
	// just an exact string match.
	_, err = s.Create("wide", "10.100.0.0/16", "bridge")
	require.NoError(t, err)

	_, err = s.Create("inner", "10.100.5.0/24", "bridge")
	require.Error(t, err)
	require.Contains(t, err.Error(), "overlaps")
}

func TestCreate_AutoAllocationSkipsOverlaps(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	// An explicit default-range network then two auto-allocated ones must all get
	// distinct, non-overlapping /24s.
	n1, err := s.Create("net1", "10.100.0.0/24", "bridge")
	require.NoError(t, err)
	n2, err := s.Create("net2", "", "bridge")
	require.NoError(t, err)
	n3, err := s.Create("net3", "", "bridge")
	require.NoError(t, err)

	subnets := map[string]bool{n1.Subnet: true}
	require.False(t, subnets[n2.Subnet], "auto subnet %s overlaps an existing network", n2.Subnet)
	subnets[n2.Subnet] = true
	require.False(t, subnets[n3.Subnet], "auto subnet %s overlaps an existing network", n3.Subnet)
}

func TestSubnetsOverlap(t *testing.T) {
	mk := func(cidr string) *net.IPNet {
		return mustCIDR(t, cidr)
	}
	require.True(t, subnetsOverlap(mk("10.0.0.0/24"), mk("10.0.0.0/24")))
	require.True(t, subnetsOverlap(mk("10.0.0.0/16"), mk("10.0.5.0/24")))
	require.True(t, subnetsOverlap(mk("10.0.5.0/24"), mk("10.0.0.0/16")))
	require.False(t, subnetsOverlap(mk("10.0.0.0/24"), mk("10.0.1.0/24")))
	require.False(t, subnetsOverlap(mk("10.0.0.0/24"), mk("192.168.0.0/24")))
}

// ---------------------------------------------------------------------------
// BUG-006: network rm left the bridge (and its route) behind. The Store now
// tears the bridge down; here we assert removal still succeeds and, being
// best-effort about a bridge that never existed, does not error in a test
// environment without one.
// ---------------------------------------------------------------------------

func TestRemove_DestroysRecordBestEffortBridge(t *testing.T) {
	s, err := NewStore(t.TempDir())
	require.NoError(t, err)
	_, err = s.Create("testnet", "10.100.0.0/24", "bridge")
	require.NoError(t, err)

	// The bridge was never created (no VM joined), so DestroyBridge fails inside
	// Remove; that must remain best-effort and not fail the removal.
	require.NoError(t, s.Remove("testnet"))
	_, err = s.Get("testnet")
	require.Error(t, err)

	// The subnet is now free for reuse without an overlap error.
	_, err = s.Create("reuse", "10.100.0.0/24", "bridge")
	require.NoError(t, err)
}
