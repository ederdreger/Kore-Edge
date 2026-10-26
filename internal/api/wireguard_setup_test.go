package api

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"kore-edge/internal/accounts"
	"kore-edge/internal/wireguard"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAuthenticatedWireGuardPreparation(t *testing.T) {
	const password = "synthetic-admin-password"
	directory := t.TempDir()
	users, e := accounts.Open(filepath.Join(directory, "users"))
	if e != nil {
		t.Fatal(e)
	}
	defer users.Close()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), 10)
	if users.BootstrapHash("owner", string(hash)) != nil {
		t.Fatal("bootstrap")
	}
	credentials, e := wireguard.OpenCredentials(filepath.Join(directory, "wg"))
	if e != nil {
		t.Fatal(e)
	}
	defer credentials.Close()
	h := (&API{Accounts: users, PublicOrigin: "https://edge.example", WireGuardCredentials: credentials}).Handler()
	var cookie *http.Cookie
	csrf := ""
	call := func(method, path string, input any, token string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(input)
		r := httptest.NewRequest(method, path, strings.NewReader(string(b)))
		r.RemoteAddr = "127.0.0.1:1234"
		r.Host = "127.0.0.1:18080"
		r.Header.Set("Origin", "https://edge.example")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", token)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/v1/wireguard/setup", nil, ""); w.Code != 401 {
		t.Fatal("anonymous credentials exposed")
	}
	login := func(name, secret string) {
		w := call("POST", "/api/v1/auth/login", map[string]any{"username": name, "password": secret}, "")
		if w.Code != 200 {
			t.Fatal("login", w.Code)
		}
		cookie = w.Result().Cookies()[0]
		var v struct {
			CSRF string `json:"csrf"`
		}
		json.Unmarshal(w.Body.Bytes(), &v)
		csrf = v.CSRF
	}
	login("owner", password)
	input := map[string]any{"current_password": password, "confirmed": true}
	if w := call("POST", "/api/v1/wireguard/setup/identity", input, ""); w.Code != 403 {
		t.Fatal("missing CSRF accepted")
	}
	if w := call("POST", "/api/v1/wireguard/setup/identity", map[string]any{"current_password": "wrong", "confirmed": true}, csrf); w.Code != 403 {
		t.Fatal("wrong password accepted")
	}
	if w := call("POST", "/api/v1/wireguard/setup/identity", map[string]any{"current_password": password}, csrf); w.Code != 400 {
		t.Fatal("unconfirmed generation")
	}
	w := call("POST", "/api/v1/wireguard/setup/identity", input, csrf)
	if w.Code != 200 {
		t.Fatal("identity generation", w.Code)
	}
	var identity wireguard.CredentialStatus
	json.Unmarshal(w.Body.Bytes(), &identity)
	if identity.PublicKey == "" || identity.Applied {
		t.Fatal("bad identity")
	}
	key, _ := ecdh.X25519().GenerateKey(rand.Reader)
	settings := wireguard.PeerSettings{Interface: "wg-edge", Address: "10.87.0.2/32", ListenPort: 51821, KorePlayPublicKey: base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), KorePlayEndpoint: "kore-play.example:51820", AllowedIPs: []string{"10.87.0.1/32"}, Keepalive: 25}
	w = call("PUT", "/api/v1/wireguard/setup/peer", map[string]any{"settings": settings, "revision": identity.Revision, "confirmed": true, "current_password": password}, csrf)
	if w.Code != 200 {
		t.Fatal("peer configuration", w.Code)
	}
	w = call("PUT", "/api/v1/wireguard/setup/peer", map[string]any{"settings": settings, "revision": identity.Revision, "confirmed": true, "current_password": password}, csrf)
	if w.Code != 409 {
		t.Fatal("stale peer configuration", w.Code)
	}
	if w = call("POST", "/api/v1/streams", map[string]any{}, csrf); w.Code != 403 {
		t.Fatal("general operational write granted")
	}
	w = call("GET", "/api/v1/wireguard/setup", nil, "")
	if w.Code != 200 || !strings.Contains(w.Body.String(), identity.PublicKey) || strings.Contains(w.Body.String(), "private_key") {
		t.Fatal("invalid public view")
	}
	if w = call("POST", "/api/v1/users", map[string]any{"username": "reader", "password": password, "role": "viewer", "current_password": password}, csrf); w.Code != 204 {
		t.Fatal("reader creation", w.Code)
	}
	login("reader", password)
	for _, path := range []string{"/api/v1/wireguard/setup", "/api/v1/wireguard/setup/identity"} {
		method := "GET"
		if strings.HasSuffix(path, "identity") {
			method = "POST"
		}
		if w = call(method, path, input, csrf); w.Code != 403 {
			t.Fatal("viewer accessed setup", w.Code)
		}
	}
	audit, _ := os.ReadFile(filepath.Join(directory, "users", "audit", "audit.jsonl"))
	if strings.Contains(string(audit), password) || strings.Contains(string(audit), identity.PublicKey) || strings.Contains(string(audit), "kore-play.example") {
		t.Fatal("audit leaked sensitive request data")
	}
}
