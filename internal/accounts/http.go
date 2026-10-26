package accounts

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"kore-edge/internal/catalog"
)

const CookieName = "__Host-kore_session"

func canonicalOrigin(raw string) string {
	u, e := url.Parse(raw)
	if e != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if strings.Contains(host, ":") {
		host = "[" + host + "]"
	}
	if p := u.Port(); p != "" && p != "443" {
		host = net.JoinHostPort(u.Hostname(), p)
	}
	return "https://" + host
}
func sessionKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}
func body(r *http.Request, dest any) bool {
	b, e := io.ReadAll(io.LimitReader(r.Body, 8193))
	return e == nil && len(b) <= 8192 && catalog.DecodeStrict(b, dest) == nil
}
func reply(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if value != nil {
		_ = json.NewEncoder(w).Encode(value)
	}
}
func (s *Store) Wrap(origin string, next http.Handler) http.Handler {
	origin = canonicalOrigin(origin)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || origin == "" {
			reply(w, 403, nil)
			return
		}
		path := r.URL.Path
		mutating := r.Method != "GET" && r.Method != "HEAD"
		if mutating {
			if r.URL.RawQuery != "" || canonicalOrigin(r.Header.Get("Origin")) != origin || r.Header.Get("Sec-Fetch-Site") == "cross-site" || r.Header.Get("Content-Type") != "application/json" {
				reply(w, 403, nil)
				return
			}
			select {
			case s.slots <- struct{}{}:
				defer func() { <-s.slots }()
			default:
				reply(w, 429, nil)
				return
			}
		}
		if path == "/api/v1/auth/login" && r.Method == "POST" {
			var input struct {
				Username string `json:"username"`
				Password string `json:"password"`
				Remember bool   `json:"remember"`
			}
			if !body(r, &input) || len(input.Password) > 72 || len(input.Username) > 128 {
				reply(w, 400, nil)
				return
			}
			raw, v, u, e := s.login(input.Username, input.Password, input.Remember)
			if e != nil {
				reply(w, 401, map[string]string{"error": "Credenciais invalidas ou limite de tentativas atingido."})
				return
			}
			cookie := &http.Cookie{Name: CookieName, Value: raw, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode}
			if input.Remember {
				cookie.Expires = v.Expires
				cookie.MaxAge = int(time.Until(v.Expires).Seconds())
			}
			http.SetCookie(w, cookie)
			reply(w, 200, map[string]any{"user": u, "csrf": v.CSRF})
			return
		}
		raw := ""
		if cookie, e := r.Cookie(CookieName); e == nil && len(cookie.Value) == 64 {
			raw = cookie.Value
		}
		v, u, ok := s.authenticate(raw)
		publicAsset := path == "/healthz" || path == "/readyz" || path == "/" || path == "/ui/" || strings.HasPrefix(path, "/ui/assets/") || strings.HasPrefix(path, "/ui/licenses/")
		if !ok {
			if publicAsset && !mutating {
				next.ServeHTTP(w, r)
				return
			}
			reply(w, 401, nil)
			return
		}
		if mutating && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(v.CSRF)) != 1 {
			reply(w, 403, nil)
			return
		}
		if path == "/api/v1/auth/me" && r.Method == "GET" {
			reply(w, 200, map[string]any{"user": u, "csrf": v.CSRF})
			return
		}
		if path == "/api/v1/session" && (r.Method == "GET" || r.Method == "HEAD") {
			reply(w, 200, map[string]any{"role": u.Role, "mode": "panel", "authenticated": true, "username": u.Username})
			return
		}
		if path == "/api/v1/auth/logout" && r.Method == "POST" {
			s.mu.Lock()
			delete(s.sessions, sessionKey(raw))
			s.mu.Unlock()
			http.SetCookie(w, &http.Cookie{Name: CookieName, Value: "", Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
			reply(w, 204, nil)
			return
		}
		if path == "/api/v1/users" && r.Method == "GET" {
			if u.Role != "admin" {
				reply(w, 403, nil)
				return
			}
			reply(w, 200, s.Users())
			return
		}
		if path == "/api/v1/users" && r.Method == "POST" || strings.HasPrefix(path, "/api/v1/users/") && r.Method == "PATCH" {
			id := ""
			if r.Method == "PATCH" {
				id = strings.TrimPrefix(path, "/api/v1/users/")
				if len(id) != 32 {
					reply(w, 400, nil)
					return
				}
			}
			var input struct {
				Username        string `json:"username"`
				Password        string `json:"password"`
				CurrentPassword string `json:"current_password"`
				Role            string `json:"role"`
				Disabled        bool   `json:"disabled"`
			}
			if !body(r, &input) {
				reply(w, 400, nil)
				return
			}
			if e := s.change(u, input.CurrentPassword, id, input.Username, input.Password, input.Role, input.Disabled); e != nil {
				reply(w, 400, map[string]string{"error": "Alteracao recusada. Confira os dados e a senha atual; mantenha um administrador ativo."})
				return
			}
			reply(w, 204, nil)
			return
		}
		// Existing operational APIs retain their existing mTLS/read-only guard.
		clone := r.Clone(context.WithValue(r.Context(), accountKey{}, u))
		clone.Header = r.Header.Clone()
		clone.Header.Del("Origin")
		next.ServeHTTP(w, clone)
	})
}

type accountKey struct{}
