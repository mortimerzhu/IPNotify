// Package dingtalk implements a notifier for DingTalk custom robots, with
// optional HMAC-SHA256 request signing.
package dingtalk

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
	"net/url"
	"strconv"
	"time"

	"github.com/mortimerzhu/IPNotify/pkg/event"
	"github.com/mortimerzhu/IPNotify/pkg/notifier"
)

func init() {
	notifier.Register("dingtalk", New)
}

// DingTalk posts text messages to a DingTalk custom robot webhook.
type DingTalk struct {
	webhook string
	secret  string
	client  *http.Client
	now     func() time.Time // injectable for tests
}

// New builds a DingTalk notifier from config: {webhook, secret?}.
func New(cfg map[string]any) (notifier.Notifier, error) {
	webhook, err := notifier.String(cfg, "webhook", true)
	if err != nil {
		return nil, err
	}
	secret, err := notifier.String(cfg, "secret", false)
	if err != nil {
		return nil, err
	}
	return &DingTalk{
		webhook: webhook,
		secret:  secret,
		client:  &http.Client{Timeout: 10 * time.Second},
		now:     time.Now,
	}, nil
}

// Name implements notifier.Notifier.
func (d *DingTalk) Name() string { return "dingtalk" }

// Notify implements notifier.Notifier.
func (d *DingTalk) Notify(ctx context.Context, e event.Event) error {
	body, err := json.Marshal(map[string]any{
		"msgtype": "text",
		"text":    map[string]string{"content": notifier.FormatText(e)},
	})
	if err != nil {
		return err
	}
	endpoint := d.sign(d.webhook)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	// DingTalk returns HTTP 200 with an errcode in the body on logical errors.
	var r struct {
		ErrCode int    `json:"errcode"`
		ErrMsg  string `json:"errmsg"`
	}
	_ = json.Unmarshal(data, &r)
	if resp.StatusCode != http.StatusOK || r.ErrCode != 0 {
		return fmt.Errorf("dingtalk: status=%d errcode=%d errmsg=%s", resp.StatusCode, r.ErrCode, r.ErrMsg)
	}
	return nil
}

// sign appends timestamp and sign query params when a secret is configured.
func (d *DingTalk) sign(webhook string) string {
	if d.secret == "" {
		return webhook
	}
	ts := strconv.FormatInt(d.now().UnixMilli(), 10)
	stringToSign := ts + "\n" + d.secret
	mac := hmac.New(sha256.New, []byte(d.secret))
	mac.Write([]byte(stringToSign))
	sign := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	sep := "?"
	if bytesContainsQuery(webhook) {
		sep = "&"
	}
	return webhook + sep + "timestamp=" + ts + "&sign=" + url.QueryEscape(sign)
}

func bytesContainsQuery(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] == '?' {
			return true
		}
	}
	return false
}
