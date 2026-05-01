//go:build linux

package network

import (
	"fmt"
	"os/exec"
	"strings"
)

// PortForward describes a single host-to-guest port forwarding rule.
type PortForward struct {
	HostPort  uint16
	GuestPort uint16
	Protocol  string
}

// SetupTAPPortForwarding adds iptables DNAT rules so that traffic arriving at
// the host on the given port maps is forwarded to guestIP via the TAP interface.
// It also enables IP forwarding in the kernel if needed.
func SetupTAPPortForwarding(tapName, guestIP string, ports []PortForward) error {
	if guestIP == "" {
		return fmt.Errorf("guest IP is required for TAP port forwarding")
	}
	for _, pm := range ports {
		proto := pm.Protocol
		if proto == "" {
			proto = "tcp"
		}
		host := fmt.Sprintf("%d", pm.HostPort)
		guest := fmt.Sprintf("%s:%d", guestIP, pm.GuestPort)

		// DNAT: rewrite destination address for incoming packets on the TAP interface.
		cmd := exec.Command("iptables", "-t", "nat", "-A", "PREROUTING",
			"-i", tapName,
			"-p", proto, "--dport", host,
			"-j", "DNAT", "--to-destination", guest)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("iptables DNAT %s/%s: %w (output: %s)", host, proto, err, strings.TrimSpace(string(out)))
		}

		// MASQUERADE: rewrite source address for packets leaving the host.
		cmd = exec.Command("iptables", "-t", "nat", "-A", "POSTROUTING",
			"-s", guestIP,
			"-j", "MASQUERADE")
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("iptables MASQUERADE %s/%s: %w (output: %s)", host, proto, err, strings.TrimSpace(string(out)))
		}
	}

	// Ensure IP forwarding is enabled (best-effort).
	_ = exec.Command("sysctl", "-w", "net.ipv4.ip_forward=1").Run()
	return nil
}

// TeardownTAPPortForwarding removes the iptables DNAT rules created by
// SetupTAPPortForwarding.
func TeardownTAPPortForwarding(tapName, guestIP string, ports []PortForward) error {
	if guestIP == "" {
		return nil
	}
	var errs []string
	for _, pm := range ports {
		proto := pm.Protocol
		if proto == "" {
			proto = "tcp"
		}
		host := fmt.Sprintf("%d", pm.HostPort)
		guest := fmt.Sprintf("%s:%d", guestIP, pm.GuestPort)

		cmd := exec.Command("iptables", "-t", "nat", "-D", "PREROUTING",
			"-i", tapName,
			"-p", proto, "--dport", host,
			"-j", "DNAT", "--to-destination", guest)
		if out, err := cmd.CombinedOutput(); err != nil {
			errs = append(errs, fmt.Sprintf("iptables DNAT delete %s: %v (%s)", host, err, strings.TrimSpace(string(out))))
		}

		cmd = exec.Command("iptables", "-t", "nat", "-D", "POSTROUTING",
			"-s", guestIP,
			"-j", "MASQUERADE")
		if out, err := cmd.CombinedOutput(); err != nil {
			errs = append(errs, fmt.Sprintf("iptables MASQUERADE delete %s: %v (%s)", host, err, strings.TrimSpace(string(out))))
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("iptables teardown: %s", strings.Join(errs, "; "))
	}
	return nil
}
