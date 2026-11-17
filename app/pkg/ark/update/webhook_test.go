package update

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNotifyUpdatePostsJSON(t *testing.T) {
	var got map[string]string
	var contentType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	if err := NotifyUpdate(context.Background(), srv.URL, "hello"); err != nil {
		t.Fatal(err)
	}
	if got["content"] != "hello" || contentType != "application/json" {
		t.Fatalf("body %v, content type %q", got, contentType)
	}
}

func TestNotifyUpdateRejectsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	if err := NotifyUpdate(context.Background(), srv.URL, "x"); err == nil || !strings.Contains(err.Error(), "400") {
		t.Fatalf("err = %v, want HTTP 400", err)
	}
}

func TestWebhookURL(t *testing.T) {
	for _, tc := range []struct {
		value, want string
		wantErr     bool
	}{
		{value: "", want: ""},
		{value: "https://discord.com/api/webhooks/1/abc", want: "https://discord.com/api/webhooks/1/abc"},
		{value: "ftp://example.com/x", wantErr: true},
		{value: "not a url", wantErr: true},
	} {
		t.Setenv("ARK_UPDATE_WEBHOOK_URL", tc.value)
		got, err := WebhookURL()
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%q: got %q, %v", tc.value, got, err)
		}
	}
}

func TestFormatAttemptMessage(t *testing.T) {
	a := UpdateAttempt{Time: time.Now(), From: "ark-server:100", To: "ark-server:200", Result: ResultRolledBack, Message: "not ready"}
	want := "host1: ARK update of asa1 rolled_back: ark-server:100 -> ark-server:200. not ready"
	if got := FormatAttemptMessage("host1", "asa1", a); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	if got := FormatAttemptMessage("h", "i", UpdateAttempt{From: "a", Result: ResultBuildFailed}); got != "h: ARK update of i build_failed: a -> -." {
		t.Fatalf("got %q", got)
	}
}
