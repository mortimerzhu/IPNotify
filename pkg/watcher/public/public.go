// Package public watches the public egress IP by polling several HTTP services
// that echo the caller's IP. It picks the value agreed on by a majority of
// reachable sources, giving resilience against a single misbehaving endpoint.
package public

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

// Config configures the public watcher.
type Config struct {
	Interval time.Duration
	Sources  []string
}

// Watcher polls public-IP sources and emits change events.
type Watcher struct {
	interval time.Duration
	sources  []string
	client   *http.Client
	hostname string
}

// New builds a public watcher.
func New(cfg Config) *Watcher {
	interval := cfg.Interval
	if interval <= 0 {
		interval = 60 * time.Second
	}
	host, _ := os.Hostname()
	return &Watcher{
		interval: interval,
		sources:  cfg.Sources,
		client:   &http.Client{Timeout: 10 * time.Second},
		hostname: host,
	}
}

// Name implements watcher.Watcher.
func (w *Watcher) Name() string { return "public" }

// Watch implements watcher.Watcher.
func (w *Watcher) Watch(ctx context.Context, out chan<- event.Event) error {
	var current string
	// Establish a baseline first so we don't report the initial discovery as a
	// change.
	if ip, ok := w.resolve(ctx); ok {
		current = ip
	}
	ticker := time.NewTicker(w.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			ip, ok := w.resolve(ctx)
			if !ok || ip == current {
				continue
			}
			e := event.Event{
				Kind:     event.KindPublic,
				Old:      nonEmpty(current),
				New:      []string{ip},
				Hostname: w.hostname,
				Time:     time.Now(),
			}
			current = ip
			select {
			case out <- e:
			case <-ctx.Done():
				return nil
			}
		}
	}
}

// resolve queries all sources and returns the IP agreed on by the most sources.
// ok is false when no source returned a valid IP.
func (w *Watcher) resolve(ctx context.Context) (ip string, ok bool) {
	votes := map[string]int{}
	for _, src := range w.sources {
		got, err := w.fetch(ctx, src)
		if err != nil {
			slog.Debug("public source failed", "source", src, "err", err)
			continue
		}
		votes[got]++
	}
	best, bestCount := "", 0
	for v, c := range votes {
		if c > bestCount {
			best, bestCount = v, c
		}
	}
	return best, bestCount > 0
}

func (w *Watcher) fetch(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 128))
	if err != nil {
		return "", err
	}
	ip := strings.TrimSpace(string(body))
	if net.ParseIP(ip) == nil {
		return "", &net.ParseError{Type: "IP address", Text: ip}
	}
	return ip, nil
}

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
