package update

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"time"
)

// WebhookURL returns ARK_UPDATE_WEBHOOK_URL ("" = alerts disabled). A value that
// isn't an http(s) URL is an error.
func WebhookURL() (string, error) {
	raw := os.Getenv("ARK_UPDATE_WEBHOOK_URL")
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("ARK_UPDATE_WEBHOOK_URL is not an http(s) URL; alerts disabled")
	}
	return raw, nil
}

// NotifyUpdate posts {"content": msg} (Discord's webhook format) to url.
func NotifyUpdate(ctx context.Context, url, msg string) error {
	body, err := json.Marshal(map[string]string{"content": msg})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return fmt.Errorf("webhook returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// FormatAttemptMessage describes a failed update for an alert.
func FormatAttemptMessage(hostname, instance string, a UpdateAttempt) string {
	to := a.To
	if to == "" {
		to = "-"
	}
	msg := fmt.Sprintf("%s: ARK update of %s %s: %s -> %s.", hostname, instance, a.Result, a.From, to)
	if a.Message != "" {
		msg += " " + a.Message
	}
	return msg
}
