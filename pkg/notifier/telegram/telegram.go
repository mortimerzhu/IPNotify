// Package telegram implements a notifier using the Telegram Bot API.
package telegram

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
	notifier.Register("telegram", New)
}

// Telegram sends messages via the Bot API sendMessage method.
type Telegram struct {
	token   string
	chatID  string
	apiBase string // defaults to https://api.telegram.org; injectable for tests
	client  *http.Client
}

// New builds a Telegram notifier from config: {token, chat_id, api_base?}.
func New(cfg map[string]any) (notifier.Notifier, error) {
	token, err := notifier.String(cfg, "token", true)
	if err != nil {
		return nil, err
	}
	chatID, err := notifier.String(cfg, "chat_id", true)
	if err != nil {
		return nil, err
	}
	apiBase, err := notifier.String(cfg, "api_base", false)
	if err != nil {
		return nil, err
	}
	if apiBase == "" {
		apiBase = "https://api.telegram.org"
	}
	return &Telegram{
		token:   token,
		chatID:  chatID,
		apiBase: apiBase,
		client:  &http.Client{Timeout: 10 * time.Second},
	}, nil
}

// Name implements notifier.Notifier.
func (t *Telegram) Name() string { return "telegram" }

// Notify implements notifier.Notifier.
func (t *Telegram) Notify(ctx context.Context, e event.Event) error {
	body, err := json.Marshal(map[string]any{
		"chat_id": t.chatID,
		"text":    notifier.FormatText(e),
	})
	if err != nil {
		return err
	}
	endpoint := fmt.Sprintf("%s/bot%s/sendMessage", t.apiBase, t.token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	var r struct {
		OK          bool   `json:"ok"`
		Description string `json:"description"`
	}
	_ = json.Unmarshal(data, &r)
	if resp.StatusCode != http.StatusOK || !r.OK {
		return fmt.Errorf("telegram: status=%d ok=%v desc=%s", resp.StatusCode, r.OK, r.Description)
	}
	return nil
}
