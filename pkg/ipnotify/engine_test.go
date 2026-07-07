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
