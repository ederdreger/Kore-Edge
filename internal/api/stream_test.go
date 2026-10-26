package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kore-edge/internal/stream"
)

func TestStreamHTTP(t *testing.T) {
	e, err := stream.New(context.Background(), func(context.Context) (*stream.Process, error) { return nil, errors.New("unavailable") }, func(id string, _ time.Time) bool { return id == "test" }, stream.DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	a := &API{Streams: e}
	for _, tc := range []struct {
		method, path, body string
		code               int
	}{
		{"GET", "/api/v1/streams", "", 200},
		{"POST", "/api/v1/streams", `{"channel_id":"denied"}`, 403},
		{"POST", "/api/v1/streams", `{"channel_id":"test","command":"DO_NOT_ECHO"}`, 400},
		{"POST", "/api/v1/streams", `{"channel_id":"test","channel_id":"test"}`, 400},
		{"POST", "/api/v1/streams", `{"channel_id":"test"}`, 202},
		{"DELETE", "/api/v1/streams/test", "", 202},
		{"DELETE", "/api/v1/streams/unknown", "", 404},
		{"PUT", "/api/v1/streams", "", 405},
		{"GET", "/api/v1/streams/test", "", 405},
		{"HEAD", "/api/v1/streams", "", 200},
	} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)))
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
		if strings.Contains(w.Body.String(), "DO_NOT_ECHO") {
			t.Fatal("echoed input")
		}
		if tc.method == "GET" && tc.code == 200 && !json.Valid(w.Body.Bytes()) {
			t.Fatal("JSON")
		}
		if tc.method == "HEAD" && w.Body.Len() > 0 {
			t.Fatal("HEAD body")
		}
	}
	w := httptest.NewRecorder()
	(&API{}).routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/streams", nil))
	if w.Code != 503 {
		t.Fatal("disabled engine")
	}
}
