package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHandler(t *testing.T) {
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	fresh := func(healthy bool) func() (Report, bool) {
		return func() (Report, bool) {
			return Report{Time: now.Add(-time.Minute), Healthy: healthy, Instances: map[string]InstanceReport{}}, true
		}
	}
	stale := func() (Report, bool) { return Report{Time: now.Add(-ReportStaleAfter - time.Second)}, true }
	missing := func() (Report, bool) { return Report{}, false }

	cases := []struct {
		name    string
		method  string
		path    string
		latest  func() (Report, bool)
		status  int
		healthy *bool
		errMsg  string
	}{
		{"fresh healthy", http.MethodGet, "/health", fresh(true), 200, new(true), ""},
		{"fresh unhealthy", http.MethodGet, "/health", fresh(false), 200, new(false), ""},
		{"stale", http.MethodGet, "/health", stale, 503, nil,
			"health report is stale (last update " + now.Add(-ReportStaleAfter-time.Second).Format(time.RFC3339) + ")"},
		{"missing", http.MethodGet, "/health", missing, 503, nil, "no health report yet"},
		{"wrong path", http.MethodGet, "/other", fresh(true), 404, nil, "not found"},
		{"post", http.MethodPost, "/health", fresh(true), 405, nil, "method not allowed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			Handler(tc.latest, func() time.Time { return now }).ServeHTTP(rec, httptest.NewRequest(tc.method, tc.path, nil))
			if rec.Code != tc.status {
				t.Fatalf("status = %d, want %d", rec.Code, tc.status)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
				t.Fatalf("Content-Type = %q", ct)
			}
			var body map[string]any
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("body %q: %v", rec.Body.String(), err)
			}
			if tc.healthy != nil && body["healthy"] != *tc.healthy {
				t.Fatalf("healthy = %v, want %v", body["healthy"], *tc.healthy)
			}
			if tc.errMsg != "" && body["error"] != tc.errMsg {
				t.Fatalf("error = %v, want %q", body["error"], tc.errMsg)
			}
		})
	}
}
