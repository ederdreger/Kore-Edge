package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kore-edge/internal/diagnostics"
	"kore-edge/internal/monitoring"
)

func TestMonitoringEndpoints(t *testing.T) {
	observer := monitoring.New(nil, nil)
	observer.Sample(time.Now().UTC(), nil, diagnostics.Metric{State: "unavailable", Source: "linux"})
	a := &API{Monitoring: observer}
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/api/v1/monitoring", 200}, {"GET", "/metrics", 200}, {"HEAD", "/metrics", 200}, {"POST", "/metrics", 405},
	} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.code {
			t.Fatal(tc.path, w.Code)
		}
		if tc.method == "GET" && tc.path == "/api/v1/monitoring" && !json.Valid(w.Body.Bytes()) {
			t.Fatal("JSON")
		}
		if tc.method == "GET" && tc.path == "/metrics" && !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain; version=0.0.4") {
			t.Fatal("Prometheus content type")
		}
		if tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
	}
	w := httptest.NewRecorder()
	(&API{}).routes().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if w.Code != 503 {
		t.Fatal("missing monitor")
	}
}
