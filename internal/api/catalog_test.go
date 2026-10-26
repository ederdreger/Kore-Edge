package api

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kore-edge/internal/catalog"
)

func TestCatalogHTTP(t *testing.T) {
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	store, e := catalog.New("partner-lab", base64.StdEncoding.EncodeToString(pub))
	if e != nil {
		t.Fatal(e)
	}
	a := &API{Catalog: store}
	now := time.Now().UTC()
	m := catalog.Manifest{Version: 1, Revision: 1, PartnerID: "partner-lab", IssuedAt: now.Add(-time.Minute), ExpiresAt: now.Add(time.Hour), Channels: []catalog.Channel{}, Grants: []catalog.Grant{}}
	payload, _ := json.Marshal(m)
	envelope, _ := json.Marshal(catalog.Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, append([]byte(catalog.Domain), payload...)))})
	for _, tc := range []struct {
		method, body string
		status       int
	}{
		{"GET", "", 503}, {"PUT", `{"private_key":"DO_NOT_ECHO"}`, 400},
		{"PUT", string(envelope), 204}, {"GET", "", 200}, {"HEAD", "", 200},
		{"POST", "", 405}, {"PUT", strings.Repeat("x", catalog.MaxBytes+1), 400}, {"GET", "", 200},
	} {
		w := httptest.NewRecorder()
		a.routes().ServeHTTP(w, httptest.NewRequest(tc.method, "/api/v1/catalog", strings.NewReader(tc.body)))
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.method, w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "DO_NOT_ECHO") {
			t.Fatal("echoed secret")
		}
		if tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD body")
		}
	}
	w := httptest.NewRecorder()
	(&API{}).routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/catalog", nil))
	if w.Code != 503 {
		t.Fatal("disabled catalog")
	}
}
