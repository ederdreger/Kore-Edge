package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"kore-edge/internal/multicast"
)

func TestMulticastPlanningHTTP(t *testing.T) {
	a := &API{MulticastInterfaces: func() ([]multicast.Interface, error) {
		return []multicast.Interface{{Name: "lab0", Index: 7, MTU: 1500, Up: true, Multicast: true, IPv4: []string{"10.250.0.2"}}}, nil
	}}
	target := multicast.Target{ChannelID: "test-pattern", Interface: "lab0", SourceIP: "10.250.0.2", Group: "239.18.10.1", Port: 12000, TTL: 1}
	b, _ := json.Marshal(target)
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/api/v1/multicast/interfaces", "", 200}, {"POST", "/api/v1/multicast/plan", string(b), 200},
		{"POST", "/api/v1/multicast/plan", `{"command":"DO_NOT_ECHO"}`, 400}, {"GET", "/api/v1/multicast/plan", "", 405},
		{"POST", "/api/v1/multicast/interfaces", "", 405}, {"HEAD", "/api/v1/multicast/interfaces", "", 200},
	} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if strings.Contains(w.Body.String(), "DO_NOT_ECHO") {
			t.Fatal("echo")
		}
		if tc.method == "HEAD" && w.Body.Len() > 0 {
			t.Fatal("HEAD body")
		}
		if tc.code == 200 && tc.method != "HEAD" && !json.Valid(w.Body.Bytes()) {
			t.Fatal("JSON")
		}
	}
}
