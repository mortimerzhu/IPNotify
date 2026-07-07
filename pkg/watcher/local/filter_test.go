package local

import (
	"net"
	"strings"
	"testing"
)

func TestKeepIP(t *testing.T) {
	tests := []struct {
		ip          string
		disableIPv6 bool
		includeULA  bool
		want        bool
	}{
		{"192.168.1.10", false, false, true}, // private IPv4 kept
		{"30.120.26.76", false, false, true}, // any global IPv4 kept
		{"127.0.0.1", false, false, false},   // loopback dropped
		{"169.254.1.1", false, false, false}, // IPv4 link-local dropped
		{"fe80::1", false, false, false},     // IPv6 link-local dropped
		{"fd00:1:6::1", false, false, false}, // IPv6 ULA dropped by default
		{"fd00:1:6::1", false, true, true},   // ULA kept when opted in
		{"2001:db8::1", false, false, true},  // global IPv6 kept
		{"2001:db8::1", true, false, false},  // all IPv6 dropped when disabled
	}
	for _, tc := range tests {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", tc.ip)
		}
		if got := keepIP(ip, tc.disableIPv6, tc.includeULA); got != tc.want {
			t.Errorf("keepIP(%s, disableIPv6=%v, includeULA=%v) = %v, want %v",
				tc.ip, tc.disableIPv6, tc.includeULA, got, tc.want)
		}
	}
}

func TestIsVirtualInterface(t *testing.T) {
	tests := []struct {
		name  string
		flags net.Flags
		want  bool
	}{
		// physical interfaces across OSes -> not virtual
		{"en0", net.FlagUp, false},      // macOS
		{"en1", net.FlagUp, false},      // macOS Wi-Fi
		{"eth0", net.FlagUp, false},     // Linux wired
		{"enp3s0", net.FlagUp, false},   // Linux predictable
		{"wlan0", net.FlagUp, false},    // Linux Wi-Fi
		{"Ethernet", net.FlagUp, false}, // Windows
		{"Wi-Fi", net.FlagUp, false},    // Windows
		// virtual -> excluded
		{"lo0", net.FlagUp | net.FlagLoopback, true},
		{"utun4", net.FlagUp | net.FlagPointToPoint, true},
		{"bridge100", net.FlagUp, true},
		{"docker0", net.FlagUp, true},
		{"br-1a2b3c", net.FlagUp, true},
		{"veth1234", net.FlagUp, true},
		{"tun0", net.FlagUp, true},
		{"vEthernet (WSL)", net.FlagUp, true},
		{"VMware Network Adapter", net.FlagUp, true},
	}
	for _, tc := range tests {
		iface := net.Interface{Name: tc.name, Flags: tc.flags}
		if got := isVirtualInterface(iface); got != tc.want {
			t.Errorf("isVirtualInterface(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestCurrentIPsExcludesULAByDefault(t *testing.T) {
	// Default config: ULA excluded. None of the returned IPs should be ULA.
	for _, s := range CurrentIPs(Config{}) {
		if isIPv6ULA(net.ParseIP(s)) {
			t.Errorf("CurrentIPs returned a ULA address by default: %s", s)
		}
		if strings.HasPrefix(s, "fe80") {
			t.Errorf("CurrentIPs returned a link-local address: %s", s)
		}
	}
}
