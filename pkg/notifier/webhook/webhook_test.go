package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

func TestNotify(t *testing.T) {
	var gotBody []byte
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotBody, _ = io.ReadAll(r.Body)
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	n, err := New(map[string]any{
		"url":     srv.URL,
		"headers": map[string]any{"Authorization": "Bearer x"},
	})
	if err != nil {
		t.Fatal(err)
	}
	e := event.Event{Kind: event.KindLocal, Interface: "en0", New: []string{"10.0.0.1"}, Time: time.Now()}
	if err := n.Notify(context.Background(), e); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if gotAuth != "Bearer x" {
		t.Errorf("Authorization = %q, want %q", gotAuth, "Bearer x")
	}
	var p payload
	if err := json.Unmarshal(gotBody, &p); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if p.Interface != "en0" || p.Text == "" {
		t.Errorf("unexpected payload: %+v", p)
	}
}

func TestNotifyErrorStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	n, _ := New(map[string]any{"url": srv.URL})
	if err := n.Notify(context.Background(), event.Event{}); err == nil {
		t.Error("expected error on 500 status")
	}
}

func TestNewRequiresURL(t *testing.T) {
	if _, err := New(map[string]any{}); err == nil {
		t.Error("expected error when url missing")
	}
}
