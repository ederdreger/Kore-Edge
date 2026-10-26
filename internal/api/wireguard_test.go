package api

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestWireguardAPI(t *testing.T) {
	b, e := os.ReadFile("../../configs/wireguard-plan.example.json")
	if e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{"POST", "/api/v1/wireguard/plan", string(b), 200},
		{"POST", "/api/v1/wireguard/plan", `{"private_key":"DO_NOT_ECHO"}`, 400},
		{"POST", "/api/v1/wireguard/plan", string(b) + "{}", 400},
		{"POST", "/api/v1/wireguard/plan", strings.Repeat("x", 65537), 400},
		{"GET", "/api/v1/wireguard/plan", "", 405},
		{"GET", "/api/v1/wireguard", "", 200},
		{"POST", "/api/v1/wireguard", "", 405},
	} {
		a := &API{}
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "DO_NOT_ECHO") {
			t.Fatal("secret echoed")
		}
		if tc.code == 200 && !json.Valid(w.Body.Bytes()) {
			t.Fatal("invalid response")
		}
	}
}
