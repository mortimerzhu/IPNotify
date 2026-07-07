package dingtalk

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
)

func TestSignAndSend(t *testing.T) {
	fixed := time.Unix(1700000000, 0)
	var gotQuery url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer srv.Close()

	n, err := New(map[string]any{"webhook": srv.URL, "secret": "topsecret"})
	if err != nil {
		t.Fatal(err)
	}
	d := n.(*DingTalk)
	d.now = func() time.Time { return fixed }

	if err := d.Notify(context.Background(), event.Event{Kind: event.KindPublic, New: []string{"1.2.3.4"}}); err != nil {
		t.Fatalf("Notify: %v", err)
	}

	ts := "1700000000000" // milliseconds
	if gotQuery.Get("timestamp") != ts {
		t.Errorf("timestamp = %q, want %q", gotQuery.Get("timestamp"), ts)
	}
	mac := hmac.New(sha256.New, []byte("topsecret"))
	mac.Write([]byte(ts + "\n" + "topsecret"))
	want := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	if gotQuery.Get("sign") != want {
		t.Errorf("sign = %q, want %q", gotQuery.Get("sign"), want)
	}
}

func TestErrCode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"errcode":300001,"errmsg":"token invalid"}`))
	}))
	defer srv.Close()
	n, _ := New(map[string]any{"webhook": srv.URL})
	if err := n.Notify(context.Background(), event.Event{}); err == nil {
		t.Error("expected error on non-zero errcode")
	}
}
