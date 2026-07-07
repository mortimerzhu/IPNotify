package notifier

import (
	"strings"
	"testing"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

func TestFormatTestEvent(t *testing.T) {
	e := event.Event{
		Kind:     event.KindLocal,
		Test:     true,
		New:      []string{"192.168.1.20", "fd00::1"},
		Hostname: "mymac",
		Time:     time.Unix(1700000000, 0).UTC(),
	}
	out := FormatText(e)
	t.Logf("\n%s", out)
	if !strings.Contains(out, "test notification") {
		t.Errorf("test event should be labelled as a test:\n%s", out)
	}
	if !strings.Contains(out, "Current IP: 192.168.1.20, fd00::1") {
		t.Errorf("test event should list current IPs:\n%s", out)
	}
	if strings.Contains(out, "Old:") {
		t.Errorf("test event should not show Old/New:\n%s", out)
	}
}

func TestFormatChangeEvent(t *testing.T) {
	e := event.Event{
		Kind:      event.KindLocal,
		Interface: "en0",
		Old:       []string{"192.168.1.10"},
		New:       []string{"192.168.1.11"},
		Time:      time.Unix(1700000000, 0).UTC(),
	}
	out := FormatText(e)
	if !strings.Contains(out, "Local IP changed (en0)") ||
		!strings.Contains(out, "Old: 192.168.1.10") ||
		!strings.Contains(out, "New: 192.168.1.11") {
		t.Errorf("unexpected change message:\n%s", out)
	}
}
