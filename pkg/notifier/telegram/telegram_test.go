package telegram

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

func TestNotify(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotBody)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	n, err := New(map[string]any{
		"token":    "123:ABC",
		"chat_id":  "42",
		"api_base": srv.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := n.Notify(context.Background(), event.Event{Kind: event.KindPublic, New: []string{"9.9.9.9"}}); err != nil {
		t.Fatalf("Notify: %v", err)
	}
	if !strings.HasSuffix(gotPath, "/bot123:ABC/sendMessage") {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["chat_id"] != "42" {
		t.Errorf("chat_id = %v, want 42", gotBody["chat_id"])
	}
	if txt, _ := gotBody["text"].(string); txt == "" {
		t.Error("empty text")
	}
}

func TestNotifyNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"ok":false,"description":"bad token"}`))
	}))
	defer srv.Close()
	n, _ := New(map[string]any{"token": "x", "chat_id": "1", "api_base": srv.URL})
	if err := n.Notify(context.Background(), event.Event{}); err == nil {
		t.Error("expected error when ok=false")
	}
}
