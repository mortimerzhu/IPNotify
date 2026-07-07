package ipnotify

import (
	"context"
	"errors"
	"testing"

	"github.com/mortimerzhu/IPNotify/pkg/event"
	"github.com/mortimerzhu/IPNotify/pkg/notifier"
)

type fakeNotifier struct {
	name string
	err  error
}

func (f *fakeNotifier) Name() string { return f.name }
func (f *fakeNotifier) Notify(ctx context.Context, e event.Event) error {
	return f.err
}

func TestTestAll(t *testing.T) {
	eng := New(nil, []notifier.Notifier{
		&fakeNotifier{name: "ok"},
		&fakeNotifier{name: "bad", err: errors.New("boom")},
	})
	results := eng.TestAll(context.Background())
	if len(results) != 2 {
		t.Fatalf("got %d results", len(results))
	}
	byName := map[string]TestResult{}
	for _, r := range results {
		byName[r.Notifier] = r
	}
	if !byName["ok"].Success {
		t.Error("ok notifier should succeed")
	}
	if byName["bad"].Success || byName["bad"].Error == "" {
		t.Error("bad notifier should report failure")
	}
}

func TestSnapshotAndSetNotifiers(t *testing.T) {
	eng := New(nil, nil)
	eng.SetNotifiers([]notifier.Notifier{&fakeNotifier{name: "a"}})
	if got := eng.snapshotNotifiers(); len(got) != 1 || got[0].Name() != "a" {
		t.Errorf("snapshotNotifiers = %v", got)
	}
	s := eng.Snapshot()
	if s.Local == nil || s.LastNotify == nil {
		t.Error("snapshot maps should be non-nil")
	}
}

// captureNotifier records the last event it received (for asserting payloads).
type captureNotifier struct {
	name string
	last event.Event
}

func (c *captureNotifier) Name() string { return c.name }
func (c *captureNotifier) Notify(ctx context.Context, e event.Event) error {
	c.last = e
	return nil
}

func TestTestAllIncludesLocalAndPublic(t *testing.T) {
	cap := &captureNotifier{name: "cap"}
	eng := New(nil, []notifier.Notifier{cap},
		WithLocalIPs(func() []string { return []string{"192.168.1.102"} }),
		WithPublicIPs(func() []string { return []string{"1.2.3.4"} }),
	)
	eng.TestAll(context.Background())
	if got := cap.last.LocalIPs; len(got) != 1 || got[0] != "192.168.1.102" {
		t.Errorf("LocalIPs = %v, want [192.168.1.102]", got)
	}
	if got := cap.last.PublicIPs; len(got) != 1 || got[0] != "1.2.3.4" {
		t.Errorf("PublicIPs = %v, want [1.2.3.4]", got)
	}
	if !cap.last.Test {
		t.Error("test event should be marked Test")
	}
}

func TestTestAllLocalOnlyOmitsPublic(t *testing.T) {
	cap := &captureNotifier{name: "cap"}
	// Only the local provider is wired (mirrors local-only config).
	eng := New(nil, []notifier.Notifier{cap},
		WithLocalIPs(func() []string { return []string{"192.168.1.102"} }),
	)
	eng.TestAll(context.Background())
	if len(cap.last.LocalIPs) == 0 {
		t.Error("local-only test should include local IPs")
	}
	if len(cap.last.PublicIPs) != 0 {
		t.Errorf("local-only test should omit WAN IPs, got %v", cap.last.PublicIPs)
	}
}

func TestTestAllPublicOnlyOmitsLocal(t *testing.T) {
	cap := &captureNotifier{name: "cap"}
	// Only the public provider is wired (mirrors WAN-only config).
	eng := New(nil, []notifier.Notifier{cap},
		WithPublicIPs(func() []string { return []string{"1.2.3.4"} }),
	)
	eng.TestAll(context.Background())
	if len(cap.last.PublicIPs) == 0 {
		t.Error("WAN-only test should include WAN IPs")
	}
	if len(cap.last.LocalIPs) != 0 {
		t.Errorf("WAN-only test should omit local IPs, got %v", cap.last.LocalIPs)
	}
}
