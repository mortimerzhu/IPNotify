package notifier

import (
	"fmt"
	"strings"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

// Title returns a short one-line summary suitable for a message title.
func Title(e event.Event) string {
	if e.Test {
		return "🔔 IPNotify test notification"
	}
	switch e.Kind {
	case event.KindPublic:
		return "🌍 Public IP changed"
	case event.KindLocal:
		if e.Interface != "" {
			return fmt.Sprintf("🔌 Local IP changed (%s)", e.Interface)
		}
		return "🔌 Local IP changed"
	default:
		return "IP changed"
	}
}

// FormatText renders an event as a plain-text message body reused by notifiers.
func FormatText(e event.Event) string {
	var b strings.Builder
	fmt.Fprintln(&b, Title(e))
	if e.Hostname != "" {
		fmt.Fprintf(&b, "Host: %s\n", e.Hostname)
	}
	if e.Interface != "" {
		fmt.Fprintf(&b, "Interface: %s\n", e.Interface)
	}
	if e.Test {
		fmt.Fprintf(&b, "Current IP: %s\n", joinOrNone(e.New))
	} else {
		fmt.Fprintf(&b, "Old: %s\n", joinOrNone(e.Old))
		fmt.Fprintf(&b, "New: %s\n", joinOrNone(e.New))
	}
	fmt.Fprintf(&b, "Time: %s", e.Time.Format("2006-01-02 15:04:05 MST"))
	return b.String()
}

func joinOrNone(ips []string) string {
	if len(ips) == 0 {
		return "(none)"
	}
	return strings.Join(ips, ", ")
}
