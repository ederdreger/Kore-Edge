package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestEdgeAndDashboard(t *testing.T) {
	handler := (&API{}).routes()
	for _, path := range []string{"/", "/ui/", "/api/v1/edge"} {
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 {
			t.Fatal(path, w.Code)
		}
		if path == "/api/v1/edge" {
			var value struct {
				OS       string          `json:"os"`
				Uptime   float64         `json:"uptime_seconds"`
				Features map[string]bool `json:"features"`
			}
			if json.Unmarshal(w.Body.Bytes(), &value) != nil || value.OS == "" || value.Uptime < 0 || value.Features["hls"] {
				t.Fatal("invented edge metadata")
			}
		}
	}
}
