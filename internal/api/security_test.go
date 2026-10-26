package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestPublicHandlerDefaultsToReadOnly(t *testing.T) {
	a := &API{}
	h := a.Handler()
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"GET", "/healthz", 200}, {"GET", "/api/v1/session", 200}, {"PUT", "/api/v1/catalog", 403}, {"POST", "/api/v1/provisioning/register", 403}, {"POST", "/api/v1/streams", 403}, {"DELETE", "/api/v1/streams/test", 403}} {
		req := httptest.NewRequest(tc.method, "http://127.0.0.1:8080"+tc.path, nil)
		req.RemoteAddr = "127.0.0.1:2345"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatalf("%s %s: got %d", tc.method, tc.path, w.Code)
		}
		if tc.path == "/api/v1/session" {
			var session map[string]any
			if e := json.Unmarshal(w.Body.Bytes(), &session); e != nil {
				t.Fatal(e)
			}
			if session["mode"] != "local_readonly" || session["authenticated"] != false || session["role"] != "viewer" || len(session) != 3 {
				t.Fatalf("unexpected session %v", session)
			}
		}
	}
}
