package api

import (
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"kore-edge/internal/accounts"
	"kore-edge/internal/edgecatalog"
	"kore-edge/internal/wireguard"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestPartnerCatalogSessionAndAdministrativeBoundary(t *testing.T) {
	const password = "synthetic-admin-password"
	dir := t.TempDir()
	users, e := accounts.Open(filepath.Join(dir, "users"))
	if e != nil {
		t.Fatal(e)
	}
	defer users.Close()
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), 10)
	if users.BootstrapHash("owner", string(hash)) != nil {
		t.Fatal("bootstrap")
	}
	credentials, e := wireguard.OpenCredentials(filepath.Join(dir, "wg"))
	if e != nil {
		t.Fatal(e)
	}
	defer credentials.Close()
	service, e := edgecatalog.New(filepath.Join(dir, "catalog"), credentials)
	if e != nil {
		t.Fatal(e)
	}
	h := (&API{Accounts: users, PublicOrigin: "https://edge.example", PartnerCatalog: service}).Handler()
	var cookie *http.Cookie
	csrf := ""
	call := func(method, path string, body any, token string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(body)
		r := httptest.NewRequest(method, path, strings.NewReader(string(b)))
		r.Host = "127.0.0.1:18080"
		r.RemoteAddr = "127.0.0.1:1234"
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
	for _, path := range []string{"catalog", "media/unlisted/http_ts/stream.ts"} {
		if w := call("GET", "/api/v1/kore-play/"+path, nil, ""); w.Code != 401 {
			t.Fatal("anonymous access", path, w.Code)
		}
	}
	login := func(name string) {
		w := call("POST", "/api/v1/auth/login", map[string]any{"username": name, "password": password}, "")
		if w.Code != 200 {
			t.Fatal("login", w.Code)
		}
		cookie = w.Result().Cookies()[0]
		var value struct {
			CSRF string `json:"csrf"`
		}
		_ = json.Unmarshal(w.Body.Bytes(), &value)
		csrf = value.CSRF
	}
	login("owner")
	if w := call("GET", "/api/v1/kore-play/catalog", nil, ""); w.Code != 200 || strings.Contains(w.Body.String(), "private_key") {
		t.Fatal("catalog session", w.Code)
	}
	for _, path := range []string{"config", "sync", "probe"} {
		if w := call("POST", "/api/v1/kore-play/"+path, map[string]any{}, ""); w.Code != 403 {
			t.Fatal("missing CSRF accepted", path, w.Code)
		}
	}
	if w := call("POST", "/api/v1/kore-play/sync", map[string]any{"confirmed": true, "current_password": "wrong"}, csrf); w.Code != 403 {
		t.Fatal("wrong password accepted", w.Code)
	}
	if w := call("POST", "/api/v1/users", map[string]any{"username": "reader", "password": password, "role": "viewer", "current_password": password}, csrf); w.Code != 204 {
		t.Fatal("viewer creation", w.Code)
	}
	login("reader")
	if w := call("GET", "/api/v1/kore-play/catalog", nil, ""); w.Code != 200 {
		t.Fatal("viewer cannot read", w.Code)
	}
	for _, path := range []string{"config", "sync", "probe"} {
		if w := call("POST", "/api/v1/kore-play/"+path, map[string]any{"confirmed": true, "current_password": password}, csrf); w.Code != 403 {
			t.Fatal("viewer mutation", path, w.Code)
		}
	}
	if w := call("GET", "/api/v1/kore-play/media/unlisted/http_ts/stream.ts", nil, ""); w.Code != 403 {
		t.Fatal("ungranted media", w.Code)
	}
}
