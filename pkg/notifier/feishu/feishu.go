// Package feishu implements a notifier for Feishu (Lark) custom robots, with
// optional HMAC-SHA256 request signing.
package feishu

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
	"github.com/mortimerzhu/IPNotify/pkg/notifier"
)

func init() {
	notifier.Register("feishu", New)
}

// Feishu posts text messages to a Feishu custom robot webhook.
type Feishu struct {
	webhook string
	secret  string
	client  *http.Client
	now     func() time.Time // injectable for tests
}

// New builds a Feishu notifier from config: {webhook, secret?}.
func New(cfg map[string]any) (notifier.Notifier, error) {
	webhook, err := notifier.String(cfg, "webhook", true)
	if err != nil {
		return nil, err
	}
	secret, err := notifier.String(cfg, "secret", false)
	if err != nil {
		return nil, err
	}
	return &Feishu{
		webhook: webhook,
		secret:  secret,
		client:  &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
	}, nil
}

// Name implements notifier.Notifier.
func (f *Feishu) Name() string { return "feishu" }

// Notify implements notifier.Notifier.
func (f *Feishu) Notify(ctx context.Context, e event.Event) error {
	payload := map[string]any{
		"msg_type": "text",
		"content":  map[string]string{"text": notifier.FormatText(e)},
	}
	if f.secret != "" {
		ts := strconv.FormatInt(f.now().Unix(), 10)
		payload["timestamp"] = ts
		payload["sign"] = f.sign(ts)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, f.webhook, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := f.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	// Feishu returns HTTP 200 with a non-zero code in the body on errors.
	var r struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	_ = json.Unmarshal(data, &r)
	if resp.StatusCode != http.StatusOK || r.Code != 0 {
		return fmt.Errorf("feishu: status=%d code=%d msg=%s", resp.StatusCode, r.Code, r.Msg)
	}
	return nil
}

// sign computes the Feishu signature: base64(HMAC-SHA256(key=ts+"\n"+secret, msg="")).
func (f *Feishu) sign(ts string) string {
	stringToSign := ts + "\n" + f.secret
	mac := hmac.New(sha256.New, []byte(stringToSign))
	// Feishu signs an empty message body with the composed key.
	mac.Write([]byte{})
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
