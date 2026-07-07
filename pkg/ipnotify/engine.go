// Package ipnotify provides the core engine that wires watchers to notifiers.
// It is the primary library entry point: aggregate watcher events, deduplicate
// them, and fan out to all notifiers concurrently.
package ipnotify

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
	"github.com/mortimerzhu/IPNotify/pkg/notifier"
	"github.com/mortimerzhu/IPNotify/pkg/watcher"
)

// Engine coordinates watchers and notifiers.
type Engine struct {
	watchers []watcher.Watcher
	log      *slog.Logger

	// notify tuning
	notifyTimeout time.Duration
	maxRetries    int
	retryBackoff  time.Duration

	mu        sync.RWMutex
	notifiers []notifier.Notifier
	state     State

	// localIPsFn, if set, supplies the current local IPs for the test
	// notification (so it honors the watcher's address filter).
	localIPsFn func() []string
	// publicIPsFn, if set, supplies the current public (WAN) IP for the test
	// notification via a live lookup (so it doesn't depend on the first poll
	// having happened yet).
	publicIPsFn func() []string
}

// State is a point-in-time snapshot of what the engine has observed. It is
// returned by Snapshot for the HTTP gateway.
type State struct {
	StartedAt  time.Time               `json:"started_at"`
	Local      map[string][]string     `json:"local"`  // interface -> IPs
	Public     []string                `json:"public"` // current public IP(s)
	LastChange map[string]time.Time    `json:"last_change"`
	LastNotify map[string]NotifyStatus `json:"last_notify"` // notifier name -> status
}

// NotifyStatus records the outcome of the most recent delivery to a notifier.
type NotifyStatus struct {
	Success bool      `json:"success"`
	Error   string    `json:"error,omitempty"`
	Time    time.Time `json:"time"`
}

// TestResult is the outcome of a synthetic test delivery to one notifier.
type TestResult struct {
	Notifier string `json:"notifier"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

// Option customizes an Engine.
type Option func(*Engine)

// WithLogger sets the logger (defaults to slog.Default()).
func WithLogger(l *slog.Logger) Option { return func(e *Engine) { e.log = l } }

// WithLocalIPs sets a provider for the current local IPs shown in the test
// notification, so it reflects the same address filter as the watcher.
func WithLocalIPs(fn func() []string) Option { return func(e *Engine) { e.localIPsFn = fn } }

// WithPublicIPs sets a provider for the current public (WAN) IP shown in the
// test notification. When unset, the test falls back to the last observed
// public IP (which is empty until the public watcher's first poll).
func WithPublicIPs(fn func() []string) Option { return func(e *Engine) { e.publicIPsFn = fn } }

// New builds an Engine.
func New(watchers []watcher.Watcher, notifiers []notifier.Notifier, opts ...Option) *Engine {
	e := &Engine{
		watchers:      watchers,
		notifiers:     notifiers,
		log:           slog.Default(),
		notifyTimeout: 15 * time.Second,
		maxRetries:    2,
		retryBackoff:  2 * time.Second,
		state: State{
			Local:      map[string][]string{},
			LastChange: map[string]time.Time{},
			LastNotify: map[string]NotifyStatus{},
		},
	}
	for _, o := range opts {
		o(e)
	}
	return e
}

// Run starts all watchers and dispatches events until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) error {
	e.mu.Lock()
	e.state.StartedAt = time.Now()
	e.mu.Unlock()

	events := make(chan event.Event, 16)

	var wg sync.WaitGroup
	for _, w := range e.watchers {
		wg.Add(1)
		go func(w watcher.Watcher) {
			defer wg.Done()
			e.log.Info("watcher started", "watcher", w.Name())
			if err := w.Watch(ctx, events); err != nil {
				e.log.Error("watcher stopped with error", "watcher", w.Name(), "err", err)
			}
		}(w)
	}

	// Close events once all watchers have exited.
	go func() {
		wg.Wait()
		close(events)
	}()

	last := map[dedupKey][]string{}
	for e2 := range events {
		if !changed(last, e2) {
			continue
		}
		e.log.Info("ip change detected",
			"kind", e2.Kind, "interface", e2.Interface, "old", e2.Old, "new", e2.New)
		e.recordChange(e2)
		e.dispatch(ctx, e2)
	}
	return ctx.Err()
}

// Snapshot returns a deep copy of the current engine state.
func (e *Engine) Snapshot() State {
	e.mu.RLock()
	defer e.mu.RUnlock()
	s := State{
		StartedAt:  e.state.StartedAt,
		Public:     append([]string(nil), e.state.Public...),
		Local:      make(map[string][]string, len(e.state.Local)),
		LastChange: make(map[string]time.Time, len(e.state.LastChange)),
		LastNotify: make(map[string]NotifyStatus, len(e.state.LastNotify)),
	}
	for k, v := range e.state.Local {
		s.Local[k] = append([]string(nil), v...)
	}
	for k, v := range e.state.LastChange {
		s.LastChange[k] = v
	}
	for k, v := range e.state.LastNotify {
		s.LastNotify[k] = v
	}
	return s
}

// SetNotifiers atomically replaces the notifier set (used by /reload).
func (e *Engine) SetNotifiers(ns []notifier.Notifier) {
	e.mu.Lock()
	e.notifiers = ns
	e.mu.Unlock()
}

// snapshotNotifiers returns the current notifier slice under lock.
func (e *Engine) snapshotNotifiers() []notifier.Notifier {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return append([]notifier.Notifier(nil), e.notifiers...)
}

// TestAll sends a synthetic event to every notifier and returns per-notifier
// results. Used by `ipnotify test` and the gateway /test endpoint.
func (e *Engine) TestAll(ctx context.Context) []TestResult {
	host, _ := os.Hostname()
	// The test notification reflects exactly which watchers are enabled: a
	// provider is only wired for a watcher that is turned on (see app.Run), so a
	// nil provider means that watcher is off and its IPs are omitted.
	var localIPs, publicIPs []string
	if e.localIPsFn != nil {
		localIPs = e.localIPsFn()
	}
	if e.publicIPsFn != nil {
		publicIPs = e.publicIPsFn()
		if len(publicIPs) == 0 {
			// WAN watching is on but the live lookup returned nothing (all
			// sources unreachable) — fall back to the last observed public IP.
			e.mu.RLock()
			publicIPs = append([]string(nil), e.state.Public...)
			e.mu.RUnlock()
		}
	}
	combined := append(append([]string(nil), localIPs...), publicIPs...)
	ev := event.Event{
		Kind:      event.KindLocal,
		Test:      true,
		New:       combined,
		LocalIPs:  localIPs,
		PublicIPs: publicIPs,
		Hostname:  host,
		Time:      time.Now(),
	}
	ns := e.snapshotNotifiers()
	results := make([]TestResult, len(ns))
	var wg sync.WaitGroup
	for i, n := range ns {
		wg.Add(1)
		go func(i int, n notifier.Notifier) {
			defer wg.Done()
			nctx, cancel := context.WithTimeout(ctx, e.notifyTimeout)
			defer cancel()
			err := n.Notify(nctx, ev)
			r := TestResult{Notifier: n.Name(), Success: err == nil}
			if err != nil {
				r.Error = err.Error()
			}
			results[i] = r
		}(i, n)
	}
	wg.Wait()
	return results
}

func (e *Engine) recordChange(ev event.Event) {
	e.mu.Lock()
	defer e.mu.Unlock()
	switch ev.Kind {
	case event.KindPublic:
		e.state.Public = append([]string(nil), ev.New...)
	case event.KindLocal:
		if len(ev.New) == 0 {
			delete(e.state.Local, ev.Interface)
		} else {
			e.state.Local[ev.Interface] = append([]string(nil), ev.New...)
		}
	}
	e.state.LastChange[changeKey(ev)] = ev.Time
}

func changeKey(ev event.Event) string {
	if ev.Kind == event.KindLocal {
		return string(ev.Kind) + "|" + ev.Interface
	}
	return string(ev.Kind)
}

type dedupKey struct {
	kind  event.Kind
	iface string
}

// changed updates the last-known state and reports whether e represents a real
// change relative to what the engine has already seen. This guards against
// duplicate events from overlapping watcher mechanisms (e.g. netlink + poll).
func changed(last map[dedupKey][]string, e event.Event) bool {
	k := dedupKey{kind: e.Kind, iface: e.Interface}
	if equal(last[k], e.New) {
		return false
	}
	last[k] = e.New
	return true
}

// dispatch delivers the event to all notifiers concurrently, each with its own
// timeout and retry loop. One notifier failing does not block the others.
func (e *Engine) dispatch(ctx context.Context, ev event.Event) {
	var wg sync.WaitGroup
	for _, n := range e.snapshotNotifiers() {
		wg.Add(1)
		go func(n notifier.Notifier) {
			defer wg.Done()
			e.deliver(ctx, n, ev)
		}(n)
	}
	wg.Wait()
}

func (e *Engine) deliver(ctx context.Context, n notifier.Notifier, ev event.Event) {
	for attempt := 0; attempt <= e.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-time.After(e.retryBackoff * time.Duration(attempt)):
			case <-ctx.Done():
				return
			}
		}
		nctx, cancel := context.WithTimeout(ctx, e.notifyTimeout)
		err := n.Notify(nctx, ev)
		cancel()
		if err == nil {
			e.log.Info("notification sent", "notifier", n.Name())
			e.recordNotify(n.Name(), nil)
			return
		}
		e.log.Warn("notification failed", "notifier", n.Name(), "attempt", attempt+1, "err", err)
		e.recordNotify(n.Name(), err)
		if ctx.Err() != nil {
			return
		}
	}
	e.log.Error("notification giving up", "notifier", n.Name())
}

func (e *Engine) recordNotify(name string, err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	st := NotifyStatus{Success: err == nil, Time: time.Now()}
	if err != nil {
		st.Error = err.Error()
	}
	e.state.LastNotify[name] = st
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
