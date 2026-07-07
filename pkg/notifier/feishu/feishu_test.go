package feishu

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

func TestSignAndSend(t *testing.T) {
	fixed := time.Unix(1700000000, 0)
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	n, err := New(map[string]any{"webhook": srv.URL, "secret": "s3cr3t"})
	if err != nil {
		t.Fatal(err)
	}
	f := n.(*Feishu)
	f.now = func() time.Time { return fixed }

	if err := f.Notify(context.Background(), event.Event{Kind: event.KindPublic, New: []string{"1.1.1.1"}}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	ts := "1700000000" // seconds
	if body["timestamp"] != ts {
		t.Errorf("timestamp = %v, want %v", body["timestamp"], ts)
	}
	mac := hmac.New(sha256.New, []byte(ts+"\n"+"s3cr3t"))
	mac.Write([]byte{})
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if body["sign"] != want {
		t.Errorf("sign = %v, want %v", body["sign"], want)
	}
}

func TestErrorCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":19021,"msg":"sign match fail"}`))
	}))
	defer srv.Close()
	n, _ := New(map[string]any{"webhook": srv.URL})
	if err := n.Notify(context.Background(), event.Event{}); err == nil {
		t.Error("expected error on non-zero code")
	}
}
