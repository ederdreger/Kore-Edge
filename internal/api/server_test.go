package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestEndpoints(t *testing.T) {
	a := &API{}
	for _, tc := range []struct {
		path, method string
		ready        bool
		code         int
	}{
		{"/healthz", "GET", false, 200}, {"/readyz", "GET", false, 503},
		{"/api/v1/diagnostics", "GET", true, 200},
		{"/api/v1/diagnostics", "POST", true, 405},
		{"/api/v1/diagnostics", "HEAD", true, 200},
		{"/readyz", "GET", true, 200}, {"/readyz", "GET", false, 503},
		{"/healthz", "POST", true, 405}, {"/missing", "GET", true, 404},
		{"/healthz/", "GET", true, 404}, {"/readyz", "HEAD", true, 200},
	} {
		a.SetReady(tc.ready)
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
		if tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
		if tc.method == "GET" && !json.Valid(w.Body.Bytes()) {
			t.Fatal("invalid JSON")
		}
	}
	s := a.Server()
	if s.ReadHeaderTimeout <= 0 || s.WriteTimeout <= 0 || s.MaxHeaderBytes <= 0 {
		t.Fatal("missing limits")
	}
}
