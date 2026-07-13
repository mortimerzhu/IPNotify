// Package event defines the shared Event type produced by watchers and
// consumed by notifiers. It lives in its own package so that the watcher and
// notifier packages can both depend on it without creating an import cycle.
package event

import "time"

// Kind identifies the category of IP that changed.
type Kind string

const (
	// KindLocal is a change to a local (LAN) interface address.
	KindLocal Kind = "local"
	// KindPublic is a change to the public egress IP.
	KindPublic Kind = "public"
)

// Event describes a single observed IP change.
type Event struct {
	Kind      Kind      `json:"kind"`
	Interface string    `json:"interface,omitempty"` // local only: network interface name
	Old       []string  `json:"old"`                 // addresses before the change
	New       []string  `json:"new"`                 // addresses after the change
	Hostname  string    `json:"hostname"`
	Time      time.Time `json:"time"`
	// Test marks a synthetic event produced by `ipnotify test` / the gateway
	// /test endpoint; it is not a real IP change.
	Test bool `json:"test,omitempty"`
	// Startup marks the one-shot announcement sent when the service starts, so
	// subscribers learn the current IP after a reboot even when it did not
	// change. Like Test it is not a real change; it carries the current IPs in
	// LocalIPs/PublicIPs.
	Startup bool `json:"startup,omitempty"`
	// LocalIPs / PublicIPs carry the current local (LAN) and public (WAN)
	// addresses for a test or startup event, so the notification can show them
	// on separate labelled lines. Only populated when Test or Startup is true.
	LocalIPs  []string `json:"local_ips,omitempty"`
	PublicIPs []string `json:"public_ips,omitempty"`
}
