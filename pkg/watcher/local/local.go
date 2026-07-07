// Package local watches local (LAN) interface addresses for changes.
//
// The default implementation polls net.Interfaces at a fixed interval, which
// works on every platform. Linux can additionally use netlink for real-time
// events (see local_linux.go); that is an optimization layered on top of the
// same snapshot/diff logic here.
package local

import (
	"context"
	"net"
	"os"
	"sort"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

// Config configures the local watcher.
type Config struct {
	Interval   time.Duration
	Interfaces []string // if non-empty, only these interface names are watched
	// DisableIPv6 drops all IPv6 addresses when true.
	DisableIPv6 bool
	// IncludeIPv6ULA includes IPv6 unique-local addresses (fc00::/7). They are
	// excluded by default because they are auto-generated and noisy.
	IncludeIPv6ULA bool
	// IncludeVirtual includes virtual interfaces (VPN tunnels, bridges, docker,
	// etc.). By default only physical wired/Wi-Fi interfaces are reported.
	// Ignored when Interfaces is set (an explicit list is always honored).
	IncludeVirtual bool
}

// Watcher polls interface addresses and emits change events.
type Watcher struct {
	interval       time.Duration
	filter         map[string]bool
	hostname       string
	disableIPv6    bool
	includeULA     bool
	includeVirtual bool
}

// New builds a local watcher.
func New(cfg Config) *Watcher {
	var filter map[string]bool
	if len(cfg.Interfaces) > 0 {
		filter = make(map[string]bool, len(cfg.Interfaces))
		for _, name := range cfg.Interfaces {
			filter[name] = true
		}
	}
	interval := cfg.Interval
	if interval <= 0 {
		interval = 10 * time.Second
	}
	host, _ := os.Hostname()
	return &Watcher{
		interval:       interval,
		filter:         filter,
		hostname:       host,
		disableIPv6:    cfg.DisableIPv6,
		includeULA:     cfg.IncludeIPv6ULA,
		includeVirtual: cfg.IncludeVirtual,
	}
}

// Name implements watcher.Watcher.
func (w *Watcher) Name() string { return "local" }

// Watch implements watcher.Watcher using periodic polling.
func (w *Watcher) Watch(ctx context.Context, out chan<- event.Event) error {
	prev := w.snapshot()
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			cur := w.snapshot()
			for _, e := range diff(prev, cur, w.hostname) {
				select {
				case out <- e:
				case <-ctx.Done():
					return nil
				}
			}
			prev = cur
		}
	}
}

// snapshot returns a map of interface name -> sorted list of IP addresses,
// restricted to the configured filter and excluding loopback/down interfaces.
func (w *Watcher) snapshot() map[string][]string {
	result := map[string][]string{}
	ifaces, err := net.Interfaces()
	if err != nil {
		return result
	}
	for _, iface := range ifaces {
		// An explicit interface list is always honored verbatim; otherwise skip
		// virtual interfaces (VPN/bridge/docker/...) unless opted in.
		if w.filter != nil {
			if !w.filter[iface.Name] {
				continue
			}
		} else if !w.includeVirtual && isVirtualInterface(iface) {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		var ips []string
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			if !keepIP(ipnet.IP, w.disableIPv6, w.includeULA) {
				continue
			}
			ips = append(ips, ipnet.IP.String())
		}
		if len(ips) > 0 {
			sort.Strings(ips)
			result[iface.Name] = ips
		}
	}
	return result
}

// diff compares two snapshots and returns one event per interface whose address
// set changed (including interfaces that appeared or disappeared).
func diff(prev, cur map[string][]string, hostname string) []event.Event {
	var events []event.Event
	seen := map[string]bool{}
	now := time.Now()
	for name, curIPs := range cur {
		seen[name] = true
		prevIPs := prev[name]
		if !equal(prevIPs, curIPs) {
			events = append(events, event.Event{
				Kind:      event.KindLocal,
				Interface: name,
				Old:       prevIPs,
				New:       curIPs,
				Hostname:  hostname,
				Time:      now,
			})
		}
	}
	for name, prevIPs := range prev {
		if !seen[name] {
			events = append(events, event.Event{
				Kind:      event.KindLocal,
				Interface: name,
				Old:       prevIPs,
				New:       nil,
				Hostname:  hostname,
				Time:      now,
			})
		}
	}
	return events
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
