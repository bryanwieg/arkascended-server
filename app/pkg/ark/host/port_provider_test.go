package host

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetchTextImplStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/fail" {
			http.Error(w, "<html>bad gateway</html>", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(" 203.0.113.7\n"))
	}))
	defer srv.Close()

	body, err := FetchTextImpl(srv.URL + "/ok")
	if err != nil || body != "203.0.113.7" {
		t.Fatalf("ok: got %q, %v", body, err)
	}
	body, err = FetchTextImpl(srv.URL + "/fail")
	if err == nil || !strings.Contains(err.Error(), "502") || body != "" {
		t.Fatalf("fail: got %q, %v; want empty body and an HTTP 502 error", body, err)
	}
}
