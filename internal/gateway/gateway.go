// Package gateway exposes a lightweight HTTP control/status surface for the
// running service: health checks, current state, a test-notification trigger,
// and notifier hot-reload.
package gateway

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/ipnotify"
)

// Gateway is an HTTP server over an ipnotify.Engine.
type Gateway struct {
	engine *ipnotify.Engine
	listen string
	log    *slog.Logger
	// reload re-reads config and swaps the engine's notifiers. May be nil.
	reload func() error
	srv    *http.Server
}

// New builds a Gateway. reload may be nil to disable the /reload endpoint.
func New(engine *ipnotify.Engine, listen string, reload func() error, log *slog.Logger) *Gateway {
	if log == nil {
		log = slog.Default()
	}
	return &Gateway{engine: engine, listen: listen, reload: reload, log: log}
}

// Run starts the HTTP server and blocks until ctx is cancelled, then shuts down.
func (g *Gateway) Run(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", g.handleHealthz)
	mux.HandleFunc("/status", g.handleStatus)
	mux.HandleFunc("/test", g.handleTest)
	mux.HandleFunc("/reload", g.handleReload)

	g.srv = &http.Server{
		Addr:              g.listen,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		g.log.Info("gateway listening", "addr", g.listen)
		if err := g.srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return g.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (g *Gateway) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

func (g *Gateway) handleStatus(w http.ResponseWriter, r *http.Request) {
	s := g.engine.Snapshot()
	resp := map[string]any{
		"started_at":  s.StartedAt,
		"uptime":      time.Since(s.StartedAt).Round(time.Second).String(),
		"local":       s.Local,
		"public":      s.Public,
		"last_change": s.LastChange,
		"last_notify": s.LastNotify,
	}
	writeJSON(w, http.StatusOK, resp)
}

func (g *Gateway) handleTest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	results := g.engine.TestAll(r.Context())
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (g *Gateway) handleReload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if g.reload == nil {
		http.Error(w, "reload not supported", http.StatusNotImplemented)
		return
	}
	if err := g.reload(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	// Watcher intervals and gateway listen address are not hot-reloaded; a
	// service restart is required for those.
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":   true,
		"note": "notifiers reloaded; restart the service to apply watcher/gateway changes",
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
