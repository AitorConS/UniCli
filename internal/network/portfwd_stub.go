//go:build !linux

package network

import "errors"

// PortForward describes a single host-to-guest port forwarding rule.
type PortForward struct {
	HostPort  uint16
	GuestPort uint16
	Protocol  string
}

// SetupTAPPortForwarding is a no-op on non-Linux platforms.
func SetupTAPPortForwarding(_, _ string, _ []PortForward) error {
	return errors.New("TAP port forwarding requires Linux")
}

// TeardownTAPPortForwarding is a no-op on non-Linux platforms.
func TeardownTAPPortForwarding(_, _ string, _ []PortForward) error {
	return nil
}
