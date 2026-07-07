package gateway

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/ipnotify"
)

// startTestGateway runs a gateway on an ephemeral port and returns its base URL.
func startTestGateway(t *testing.T, engine *ipnotify.Engine, reload func() error) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()

	g := New(engine, addr, reload, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() { _ = g.Run(ctx) }()

	// wait for the listener to come up
	base := "http://" + addr
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if resp, err := http.Get(base + "/healthz"); err == nil {
			resp.Body.Close()
			return base
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("gateway did not start")
	return ""
}

func TestHealthzAndStatus(t *testing.T) {
	engine := ipnotify.New(nil, nil)
	base := startTestGateway(t, engine, nil)

	resp, err := http.Get(base + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || string(body) != "ok" {
		t.Errorf("healthz = %d %q", resp.StatusCode, body)
	}

	resp, err = http.Get(base + "/status")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var status map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if _, ok := status["uptime"]; !ok {
		t.Errorf("status missing uptime: %v", status)
	}
}

func TestReload(t *testing.T) {
	engine := ipnotify.New(nil, nil)
	called := false
	base := startTestGateway(t, engine, func() error { called = true; return nil })

	resp, err := http.Post(base+"/reload", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 || !called {
		t.Errorf("reload status=%d called=%v", resp.StatusCode, called)
	}

	// GET should be rejected.
	resp, err = http.Get(base + "/reload")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("GET /reload = %d, want 405", resp.StatusCode)
	}
}

func TestReloadUnsupported(t *testing.T) {
	engine := ipnotify.New(nil, nil)
	base := startTestGateway(t, engine, nil) // nil reload
	resp, err := http.Post(base+"/reload", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotImplemented {
		t.Errorf("reload without callback = %d, want 501", resp.StatusCode)
	}
}
