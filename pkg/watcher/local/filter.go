package local

import (
	"net"
	"sort"
	"strings"
)

// virtualPrefixes matches interface names (lower-cased) that are virtual on
// Linux/macOS/OpenWrt: VPN tunnels, bridges, container/VM adapters, etc.
var virtualPrefixes = []string{
	"lo", "docker", "veth", "br-", "bridge", "virbr", "vmnet", "vboxnet", "vnic",
	"tap", "tun", "utun", "gif", "stf", "awdl", "llw", "ipsec", "ppp",
	"wg", "zt", "tailscale", "kube", "cni", "flannel", "cali", "nomad",
}

// virtualSubstrings matches Windows-style friendly names.
var virtualSubstrings = []string{
	"virtual", "vmware", "vbox", "hyper-v", "loopback", "pseudo",
	"tap-windows", "bluetooth", "wsl", "tunnel",
}

// isVirtualInterface reports whether iface is a virtual (non-physical)
// interface using a cross-platform heuristic: loopback/point-to-point flags
// plus a name deny-list. Physical wired/Wi-Fi interfaces are not matched.
func isVirtualInterface(iface net.Interface) bool {
	if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagPointToPoint != 0 {
		return true
	}
	name := strings.ToLower(iface.Name)
	for _, p := range virtualPrefixes {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	for _, s := range virtualSubstrings {
		if strings.Contains(name, s) {
			return true
		}
	}
	return false
}

// keepIP reports whether an address should be reported. It always drops
// loopback and link-local addresses, and applies the IPv6 policy from config.
func keepIP(ip net.IP, disableIPv6, includeULA bool) bool {
	if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip.To4() == nil { // IPv6
		if disableIPv6 {
			return false
		}
		if !includeULA && isIPv6ULA(ip) {
			return false
		}
	}
	return true
}

// isIPv6ULA reports whether ip is an IPv6 unique-local address (fc00::/7).
func isIPv6ULA(ip net.IP) bool {
	ip16 := ip.To16()
	return ip16 != nil && ip.To4() == nil && ip16[0]&0xfe == 0xfc
}

// CurrentIPs returns the machine's current addresses that pass the config's
// filter, flattened and de-duplicated across interfaces. Used for the test
// notification so it reflects exactly what the watcher would report.
func CurrentIPs(cfg Config) []string {
	w := New(cfg)
	seen := map[string]bool{}
	var out []string
	for _, ips := range w.snapshot() {
		for _, ip := range ips {
			if !seen[ip] {
				seen[ip] = true
				out = append(out, ip)
			}
		}
	}
	sort.Strings(out)
	return out
}
