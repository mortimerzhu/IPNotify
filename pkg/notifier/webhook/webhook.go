// Package webhook implements a generic notifier that POSTs the event as JSON
// to an arbitrary URL with optional custom headers.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
	"github.com/mortimerzhu/IPNotify/pkg/notifier"
)

func init() {
	notifier.Register("webhook", New)
}

// Webhook posts events as JSON to a configured URL.
type Webhook struct {
	url     string
	headers map[string]string
	client  *http.Client
}

// New builds a Webhook notifier from config: {url, headers}.
func New(cfg map[string]any) (notifier.Notifier, error) {
	url, err := notifier.String(cfg, "url", true)
	if err != nil {
		return nil, err
	}
	headers, err := notifier.StringMap(cfg, "headers")
	if err != nil {
		return nil, err
	}
	return &Webhook{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Name implements notifier.Notifier.
func (w *Webhook) Name() string { return "webhook" }

// payload is the JSON body sent to the endpoint: the raw event plus a
// pre-rendered text summary for convenience.
type payload struct {
	event.Event
	Text string `json:"text"`
}

// Notify implements notifier.Notifier.
func (w *Webhook) Notify(ctx context.Context, e event.Event) error {
	body, err := json.Marshal(payload{Event: e, Text: notifier.FormatText(e)})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range w.headers {
		req.Header.Set(k, v)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("webhook: unexpected status %d: %s", resp.StatusCode, snippet)
	}
	return nil
}
