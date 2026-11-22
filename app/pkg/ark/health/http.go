package health

import (
	"encoding/json"
	"net/http"
	"time"
)

// Handler serves GET /health: 200 + Report JSON when latest returns a report newer than ReportStaleAfter;
// 503 + {"error": ...} when there is no report yet or it is stale. Other paths 404, other methods on
// /health 405. Overall health is the report's healthy field, never the status code (design 12).
func Handler(latest func() (Report, bool), now func() time.Time) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path != "/health":
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		case r.Method != http.MethodGet:
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		default:
			report, ok := latest()
			switch {
			case !ok:
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no health report yet"})
			case now().Sub(report.Time) > ReportStaleAfter:
				writeJSON(w, http.StatusServiceUnavailable, map[string]string{
					"error": "health report is stale (last update " + report.Time.Format(time.RFC3339) + ")",
				})
			default:
				writeJSON(w, http.StatusOK, report)
			}
		}
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // the client went away; nothing to do
}
