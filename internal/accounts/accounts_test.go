package accounts

import (
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"kore-edge/internal/security"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAnonymousModuleOriginBehindGateway(t *testing.T) {
	s := fixture(t)
	h := s.Wrap("https://edge.example", security.Local().Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != "" {
			t.Error("public origin forwarded to private guard")
		}
		w.WriteHeader(204)
	})))
	for _, tc := range []struct {
		name, path, origin, site, host string
		status                         int
	}{
		{"module script", "/ui/assets/app.js", "https://edge.example", "same-origin", "127.0.0.1:18080", 204},
		{"stylesheet", "/ui/assets/app.css", "", "same-origin", "127.0.0.1:18080", 204},
		{"hostile origin", "/ui/assets/app.js", "https://evil.example", "same-origin", "127.0.0.1:18080", 403},
		{"cross site", "/ui/assets/app.js", "https://edge.example", "cross-site", "127.0.0.1:18080", 403},
		{"private API", "/api/v1/edge", "https://edge.example", "same-origin", "127.0.0.1:18080", 401},
		{"untrusted host", "/ui/assets/app.js", "https://edge.example", "same-origin", "evil.example", 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", tc.path, nil)
			r.RemoteAddr = "127.0.0.1:1234"
			r.Host = tc.host
			r.Header.Set("Origin", tc.origin)
			r.Header.Set("Sec-Fetch-Site", tc.site)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != tc.status {
				t.Fatalf("status=%d want=%d", w.Code, tc.status)
			}
			if r.Header.Get("Origin") != tc.origin {
				t.Fatal("original request mutated")
			}
		})
	}
}

const testPassword = "synthetic-password-42"

func fixture(t *testing.T) *Store {
	t.Helper()
	s, e := Open(filepath.Join(t.TempDir(), "users"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	hash, e := bcrypt.GenerateFromPassword([]byte(testPassword), 10)
	if e != nil || s.BootstrapHash("admin", string(hash)) != nil {
		t.Fatal("bootstrap")
	}
	return s
}
func TestChangesRolesAndRevocation(t *testing.T) {
	s := fixture(t)
	raw, _, admin, e := s.login("ADMIN", testPassword, false)
	if e != nil {
		t.Fatal(e)
	}
	if s.change(admin, "incorrect", "", "viewer", testPassword, "viewer", false) == nil {
		t.Fatal("missing confirmation accepted")
	}
	if s.change(admin, testPassword, admin.ID, "", "", "viewer", false) == nil {
		t.Fatal("last admin demoted")
	}
	if e = s.change(admin, testPassword, "", "reader", testPassword, "viewer", false); e != nil {
		t.Fatal(e)
	}
	if s.change(admin, testPassword, "", "READER", testPassword, "viewer", false) == nil {
		t.Fatal("duplicate accepted")
	}
	readerRaw, _, reader, e := s.login("reader", testPassword, false)
	if e != nil {
		t.Fatal(e)
	}
	if s.change(reader, testPassword, "", "intruder", testPassword, "admin", false) == nil {
		t.Fatal("reader created admin")
	}
	if s.change(reader, testPassword, reader.ID, "", "", "admin", false) == nil {
		t.Fatal("reader escalated")
	}
	if e = s.change(reader, testPassword, reader.ID, "", "new-synthetic-secret", "viewer", false); e != nil {
		t.Fatal(e)
	}
	if _, _, ok := s.authenticate(readerRaw); ok {
		t.Fatal("old session survived password change")
	}
	if _, _, _, e = s.login("reader", testPassword, false); e == nil {
		t.Fatal("old password accepted")
	}
	if _, _, _, e = s.login("reader", "new-synthetic-secret", false); e != nil {
		t.Fatal(e)
	}
	if e = s.change(admin, testPassword, reader.ID, "", "", "viewer", true); e != nil {
		t.Fatal(e)
	}
	if _, _, _, e = s.login("reader", "new-synthetic-secret", false); e == nil {
		t.Fatal("disabled login")
	}
	if _, _, ok := s.authenticate(raw); !ok {
		t.Fatal("other account revoked")
	}
	data, _ := os.ReadFile(filepath.Join(s.directory, "users.json"))
	if strings.Contains(string(data), testPassword) {
		t.Fatal("plaintext stored")
	}
	safe, _ := json.Marshal(s.Users())
	if strings.Contains(string(safe), "hash") {
		t.Fatal("hash exposed")
	}
	audit, _ := os.ReadFile(filepath.Join(s.directory, "audit", "audit.jsonl"))
	if strings.Contains(string(audit), testPassword) || strings.Contains(string(audit), "reader") {
		t.Fatal("sensitive audit")
	}
}
func TestLimitsExpiryAndPersistence(t *testing.T) {
	s := fixture(t)
	directory := s.directory
	if _, e := Open(directory); e == nil {
		t.Fatal("concurrent writer")
	}
	raw, _, _, e := s.login("admin", testPassword, true)
	if e != nil {
		t.Fatal(e)
	}
	v := s.sessions[sessionKey(raw)]
	v.Expires = time.Now().Add(-time.Second)
	s.sessions[sessionKey(raw)] = v
	if _, _, ok := s.authenticate(raw); ok {
		t.Fatal("expired session")
	}
	for i := 0; i < 5; i++ {
		s.login("admin", "wrong", false)
	}
	if _, _, _, e = s.login("admin", testPassword, false); e == nil {
		t.Fatal("rate limit")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	reopened, e := Open(directory)
	if e != nil {
		t.Fatal(e)
	}
	defer reopened.Close()
	if len(reopened.Users()) != 1 {
		t.Fatal("persistence")
	}
	if _, _, _, e = reopened.login("admin", testPassword, false); e != nil {
		t.Fatal(e)
	}
	if reopened.BootstrapHash("other", s.db.Users[0].Hash) == nil {
		t.Fatal("bootstrap overwrote database")
	}
}
func TestHTTPBoundary(t *testing.T) {
	s := fixture(t)
	h := s.Wrap("https://edge.example", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(202) }))
	call := func(method, path, body, origin, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		if method != "GET" {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Origin", origin)
			r.Header.Set("X-CSRF-Token", csrf)
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if call("GET", "/api/v1/edge", "", "", "", nil).Code != 401 {
		t.Fatal("private API exposed")
	}
	if call("GET", "/", "", "", "", nil).Code != 202 {
		t.Fatal("login assets blocked")
	}
	if call("GET", "/readyz", "", "", "", nil).Code != 202 {
		t.Fatal("readiness blocked")
	}
	input := `{"username":"admin","password":"` + testPassword + `","remember":true}`
	if call("POST", "/api/v1/auth/login", input, "https://evil.example", "", nil).Code != 403 {
		t.Fatal("cross origin login")
	}
	w := call("POST", "/api/v1/auth/login", input, "https://edge.example", "", nil)
	if w.Code != 200 {
		t.Fatal(w.Code)
	}
	cookie := w.Result().Cookies()[0]
	if !cookie.Secure || !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode || cookie.Domain != "" || cookie.Path != "/" || cookie.MaxAge <= 0 {
		t.Fatal("unsafe cookie")
	}
	var response struct {
		CSRF string `json:"csrf"`
	}
	json.Unmarshal(w.Body.Bytes(), &response)
	if call("POST", "/api/v1/auth/logout", "{}", "https://edge.example", "", cookie).Code != 403 {
		t.Fatal("missing CSRF")
	}
	if call("GET", "/api/v1/users", "", "", "", cookie).Code != 200 {
		t.Fatal("admin users")
	}
	if call("POST", "/api/v1/auth/logout", "{}", "https://edge.example", response.CSRF, cookie).Code != 204 {
		t.Fatal("logout")
	}
	if call("GET", "/api/v1/auth/me", "", "", "", cookie).Code != 401 {
		t.Fatal("logout session valid")
	}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "198.51.100.1:1234"
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("public backend accepted")
	}
}
func TestOfflineRecovery(t *testing.T) {
	s := fixture(t)
	raw, _, _, _ := s.login("admin", testPassword, false)
	hash, _ := bcrypt.GenerateFromPassword([]byte("recovery-synthetic-password"), 10)
	if s.ResetHash("missing", string(hash)) == nil {
		t.Fatal("recovery created account")
	}
	if s.ResetHash("admin", string(hash)) != nil {
		t.Fatal("recovery")
	}
	if _, _, ok := s.authenticate(raw); ok {
		t.Fatal("recovery retained session")
	}
	if _, _, _, e := s.login("admin", "recovery-synthetic-password", false); e != nil {
		t.Fatal(e)
	}
}
func TestAuditFailureBlocksAccountChanges(t *testing.T) {
	s := fixture(t)
	_, _, admin, e := s.login("admin", testPassword, false)
	if e != nil {
		t.Fatal(e)
	}
	s.audit.Close()
	if s.change(admin, testPassword, "", "reader", testPassword, "viewer", false) == nil {
		t.Fatal("unaudited change accepted")
	}
	if len(s.Users()) != 1 {
		t.Fatal("unaudited change committed")
	}
}
