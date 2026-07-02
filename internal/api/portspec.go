package api

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ParsePortMaps parses a list of "host:guest[/tcp|udp]" specs into wire port
// maps. It is a client-side helper so the CLI can build RunParams without
// importing the daemon's vm package.
func ParsePortMaps(specs []string) ([]PortMapSpec, error) {
	out := make([]PortMapSpec, 0, len(specs))
	for _, s := range specs {
		pm, err := ParsePortMap(s)
		if err != nil {
			return nil, err
		}
		out = append(out, pm)
	}
	return out, nil
}

// ParsePortMap parses a single "[bindaddr:]host:guest[/tcp|udp]" port spec. The
// protocol defaults to tcp. An optional leading IPv4 bind address restricts the
// published port to that host address (e.g. "127.0.0.1:8080:80"); without it the
// port is published on all interfaces.
func ParsePortMap(s string) (PortMapSpec, error) {
	proto := "tcp"
	if idx := strings.LastIndex(s, "/"); idx >= 0 {
		p := strings.ToLower(s[idx+1:])
		if p != "tcp" && p != "udp" {
			return PortMapSpec{}, fmt.Errorf("port map %q: unknown protocol %q (want tcp or udp)", s, p)
		}
		proto = p
		s = s[:idx]
	}
	bindAddr, hostSpec, guestSpec, err := splitPortFields(s)
	if err != nil {
		return PortMapSpec{}, err
	}
	host, err := parsePortNum(hostSpec)
	if err != nil {
		return PortMapSpec{}, fmt.Errorf("port map %q: host port: %w", s, err)
	}
	guest, err := parsePortNum(guestSpec)
	if err != nil {
		return PortMapSpec{}, fmt.Errorf("port map %q: guest port: %w", s, err)
	}
	return PortMapSpec{HostPort: host, GuestPort: guest, Protocol: proto, BindAddr: bindAddr}, nil
}

// splitPortFields splits a "host:guest" or "bindaddr:host:guest" spec (protocol
// already stripped) into its parts, validating a present bind address as an IP.
func splitPortFields(s string) (bindAddr, host, guest string, err error) {
	parts := strings.Split(s, ":")
	switch len(parts) {
	case 2:
		return "", parts[0], parts[1], nil
	case 3:
		if net.ParseIP(parts[0]) == nil {
			return "", "", "", fmt.Errorf("port map %q: bind address %q is not a valid IP", s, parts[0])
		}
		return parts[0], parts[1], parts[2], nil
	default:
		return "", "", "", fmt.Errorf("port map %q: expected [bindaddr:]host:guest format", s)
	}
}

func parsePortNum(s string) (uint16, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 16)
	if err != nil {
		return 0, fmt.Errorf("invalid port %q", s)
	}
	if n == 0 {
		return 0, fmt.Errorf("port %q must be greater than 0", s)
	}
	return uint16(n), nil
}
