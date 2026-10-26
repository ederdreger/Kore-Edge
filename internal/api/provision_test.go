package api

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kore-edge/internal/provision"
)

func TestProvisioningHTTP(t *testing.T) {
	registry, e := provision.Open(filepath.Join(t.TempDir(), "registry"))
	if e != nil {
		t.Fatal(e)
	}
	pub, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	credential, e := registry.Issue(context.Background(), provision.Assignment{EdgeID: "edge-lab", PartnerID: "partner-lab", CatalogPublicKey: base64.StdEncoding.EncodeToString(pub), TTLSeconds: 60}, time.Now().UTC())
	if e != nil {
		t.Fatal(e)
	}
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	public, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	nonce := make([]byte, 32)
	if _, e = rand.Read(nonce); e != nil {
		t.Fatal(e)
	}
	proof := provision.Proof{Version: 1, EdgeID: "edge-lab", PublicKey: base64.StdEncoding.EncodeToString(public), Challenge: base64.RawURLEncoding.EncodeToString(nonce)}
	payload, _ := json.Marshal(proof)
	digest := sha256.Sum256(append([]byte(provision.Domain), payload...))
	sig, e := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if e != nil {
		t.Fatal(e)
	}
	body, _ := json.Marshal(provision.Request{Proof: proof, Signature: base64.StdEncoding.EncodeToString(sig)})
	handler := (&API{Provision: registry, ProvisionEdgeID: "edge-lab"}).routes()
	for _, tc := range []struct {
		method, path, body string
		authorized         bool
		code               int
	}{
		{"GET", "/api/v1/provisioning", "", false, 200},
		{"POST", "/api/v1/provisioning/register", string(body), false, 403},
		{"POST", "/api/v1/provisioning/register?token=DO_NOT_ECHO", string(body), true, 400},
		{"POST", "/api/v1/provisioning/register", `{"token":"DO_NOT_ECHO"}`, true, 400},
		{"POST", "/api/v1/provisioning/register", strings.Repeat("x", 4097), true, 400},
		{"POST", "/api/v1/provisioning/register", string(body), true, 201},
		{"POST", "/api/v1/provisioning/register", string(body), true, 403},
		{"GET", "/api/v1/provisioning", "", false, 200},
		{"HEAD", "/api/v1/provisioning", "", false, 200},
		{"GET", "/api/v1/provisioning/register", "", false, 405},
		{"PUT", "/api/v1/provisioning", "", false, 405},
	} {
		request := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		if tc.authorized {
			request.Header.Set("Authorization", "Bearer "+credential.Token)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, request)
		if w.Code != tc.code {
			t.Fatalf("%s %s: %d", tc.method, strings.Split(tc.path, "?")[0], w.Code)
		}
		if bytes.Contains(w.Body.Bytes(), []byte(credential.Token)) || strings.Contains(w.Body.String(), "DO_NOT_ECHO") || strings.Contains(w.Body.String(), "digest") {
			t.Fatal("credential echoed")
		}
		if tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD")
		}
	}
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/edge", nil))
	if !strings.Contains(w.Body.String(), `"enrollment_state":"registered"`) || !strings.Contains(w.Body.String(), `"edge_id":"edge-lab"`) {
		t.Fatal("identity not integrated")
	}
	w = httptest.NewRecorder()
	(&API{}).routes().ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/provisioning", nil))
	if w.Code != 503 {
		t.Fatal("provisioning enabled by default")
	}
}
