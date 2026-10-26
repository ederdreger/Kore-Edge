package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"kore-edge/internal/catalog"
	"kore-edge/internal/integration"
	"net/http/httptest"
	"testing"
)

func TestIntegrationStatusIsReadOnlyAndDisabledByDefault(t *testing.T) {
	a := &API{}
	for _, tc := range []struct {
		method string
		status int
	}{{"GET", 200}, {"HEAD", 200}, {"POST", 403}} {
		req := httptest.NewRequest(tc.method, "http://localhost:8080/api/v1/integration", nil)
		req.RemoteAddr = "127.0.0.1:2345"
		w := httptest.NewRecorder()
		a.Handler().ServeHTTP(w, req)
		if w.Code != tc.status {
			t.Fatal(w.Code)
		}
		if tc.method == "GET" {
			var st map[string]any
			if json.Unmarshal(w.Body.Bytes(), &st) != nil || st["mode"] != "disabled" || st["state"] != "disabled" || st["catalog_available"] != false {
				t.Fatal("invented integration state")
			}
		}
	}
}

func TestSynchronizedCatalogRejectsManualWriter(t *testing.T) {
	public, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	store, e := catalog.New("partner-lab", base64.StdEncoding.EncodeToString(public))
	if e != nil {
		t.Fatal(e)
	}
	a := &API{Catalog: store, Integration: &integration.Service{}}
	w := httptest.NewRecorder()
	a.routes().ServeHTTP(w, httptest.NewRequest("PUT", "/api/v1/catalog", nil))
	if w.Code != 409 {
		t.Fatal("manual writer accepted", w.Code)
	}
}
