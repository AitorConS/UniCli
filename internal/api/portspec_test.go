package api

import (
	"net"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParsePortMap(t *testing.T) {
	tests := []struct {
		name    string
		spec    string
		want    PortMapSpec
		wantErr bool
	}{
		{"tcp default", "8080:80", PortMapSpec{HostPort: 8080, GuestPort: 80, Protocol: "tcp"}, false},
		{"explicit tcp", "8080:80/tcp", PortMapSpec{HostPort: 8080, GuestPort: 80, Protocol: "tcp"}, false},
		{"bind addr", "127.0.0.1:8080:80", PortMapSpec{HostPort: 8080, GuestPort: 80, Protocol: "tcp", BindAddr: "127.0.0.1"}, false},
		{"bind addr udp", "127.0.0.1:53:53/udp", PortMapSpec{HostPort: 53, GuestPort: 53, Protocol: "udp", BindAddr: "127.0.0.1"}, false},
		{"invalid bind addr", "notanip:8080:80", PortMapSpec{}, true},
		{"too many fields", "1:2:3:4", PortMapSpec{}, true},
		{"udp", "53:53/udp", PortMapSpec{HostPort: 53, GuestPort: 53, Protocol: "udp"}, false},
		{"uppercase proto", "9000:90/UDP", PortMapSpec{HostPort: 9000, GuestPort: 90, Protocol: "udp"}, false},
		{"max port", "65535:65535", PortMapSpec{HostPort: 65535, GuestPort: 65535, Protocol: "tcp"}, false},
		{"unknown proto", "8080:80/sctp", PortMapSpec{}, true},
		{"missing colon", "8080", PortMapSpec{}, true},
		{"zero host", "0:80", PortMapSpec{}, true},
		{"zero guest", "80:0", PortMapSpec{}, true},
		{"overflow host", "70000:80", PortMapSpec{}, true},
		{"non-numeric host", "abc:80", PortMapSpec{}, true},
		{"non-numeric guest", "80:abc", PortMapSpec{}, true},
		{"empty", "", PortMapSpec{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParsePortMap(tt.spec)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestParsePortMaps(t *testing.T) {
	got, err := ParsePortMaps([]string{"8080:80", "53:53/udp"})
	require.NoError(t, err)
	require.Equal(t, []PortMapSpec{
		{HostPort: 8080, GuestPort: 80, Protocol: "tcp"},
		{HostPort: 53, GuestPort: 53, Protocol: "udp"},
	}, got)
}

func TestParsePortMaps_Empty(t *testing.T) {
	got, err := ParsePortMaps(nil)
	require.NoError(t, err)
	require.Empty(t, got)
}

func TestParsePortMaps_PropagatesError(t *testing.T) {
	_, err := ParsePortMaps([]string{"8080:80", "bad"})
	require.Error(t, err)
}

// FuzzParsePortMap asserts the parser never panics on arbitrary CLI input and,
// crucially, that every accepted spec yields a well-formed result: ports in
// [1,65535], a known protocol, and a bind address that is empty or a valid IP.
// This pins the output contract so a future regression (e.g. accepting port 0
// or an unknown protocol) cannot slip through.
func FuzzParsePortMap(f *testing.F) {
	for _, s := range []string{
		"8080:80", "8080:80/tcp", "127.0.0.1:8080:80", "53:53/udp",
		"", "notanip:8080:80", "1:2:3:4", "0:80", "70000:80", "abc:80",
		"8080:80/sctp", ":", "::", "/udp",
	} {
		f.Add(s)
	}

	f.Fuzz(func(t *testing.T, spec string) {
		pm, err := ParsePortMap(spec)
		if err != nil {
			return // rejected input carries no guarantee
		}
		if pm.HostPort == 0 || pm.GuestPort == 0 {
			t.Fatalf("ParsePortMap(%q) accepted a zero port: %+v", spec, pm)
		}
		if pm.Protocol != "tcp" && pm.Protocol != "udp" {
			t.Fatalf("ParsePortMap(%q) accepted unknown protocol %q", spec, pm.Protocol)
		}
		if pm.BindAddr != "" && net.ParseIP(pm.BindAddr) == nil {
			t.Fatalf("ParsePortMap(%q) accepted invalid bind address %q", spec, pm.BindAddr)
		}
	})
}
