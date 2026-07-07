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
	"regexp"
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

// CurrentIP resolves the public (WAN) IP once, using the given sources
// (majority vote). Returns nil when no source responds. Used by the test
// notification so it shows a live value instead of waiting for the first poll.
func CurrentIP(ctx context.Context, sources []string) []string {
	w := New(Config{Sources: sources})
	if ip, ok := w.resolve(ctx); ok {
		return []string{ip}
	}
	return nil
}

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
	body, err := io.ReadAll(io.LimitReader(resp.Body, 65536))
	if err != nil {
		return "", err
	}
	// Extract the IP by regex rather than parsing the whole body: several echo
	// services (e.g. myip.ipip.net, ddns.oray.com) wrap the address in text.
	ip := ipv4Regex.FindString(string(body))
	if ip == "" || net.ParseIP(ip) == nil {
		return "", &net.ParseError{Type: "IPv4 address", Text: string(body)}
	}
	return ip, nil
}

// ipv4Regex matches a dotted-quad IPv4 address anywhere in a response body.
var ipv4Regex = regexp.MustCompile(`\b((25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\.){3}(25[0-5]|2[0-4][0-9]|[01]?[0-9][0-9]?)\b`)

func nonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return []string{s}
}
