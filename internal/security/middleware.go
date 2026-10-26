package security

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type principalKey struct{}
type Principal struct {
	ID            string `json:"-"`
	Role          string `json:"role"`
	Authenticated bool   `json:"authenticated"`
	Mode          string `json:"mode"`
}

func PrincipalFrom(ctx context.Context) Principal {
	p, _ := ctx.Value(principalKey{}).(Principal)
	return p
}
func loopbackHost(hostport string) bool {
	host := hostport
	if h, _, e := net.SplitHostPort(hostport); e == nil {
		host = h
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}
func localRequest(r *http.Request) bool {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || !loopbackHost(r.Host) {
		return false
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		o, e := url.Parse(origin)
		scheme := "http"
		if r.TLS != nil {
			scheme = "https"
		}
		if e != nil || o.Scheme != scheme || !strings.EqualFold(o.Host, r.Host) || o.User != nil || o.Path != "" {
			return false
		}
	}
	return r.Header.Get("Sec-Fetch-Site") != "cross-site"
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	if w.status != 0 {
		return
	}
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
func (w *statusWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.WriteHeader(200)
	}
	return w.ResponseWriter.Write(p)
}
func (c *Controller) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		if c.mode == "mtls" {
			w.Header().Set("Strict-Transport-Security", "max-age=86400")
		}
		select {
		case c.slots <- struct{}{}:
			defer func() { <-c.slots }()
		default:
			w.WriteHeader(429)
			return
		}
		route := routeName(r.URL.Path)
		principal := Principal{Role: "viewer", Mode: c.mode}
		event := Event{Time: time.Now().UTC(), Route: route, Method: r.Method, PrincipalID: "anonymous", Role: "none"}
		var nonce [8]byte
		if _, e := rand.Read(nonce[:]); e != nil {
			w.WriteHeader(503)
			return
		}
		event.RequestID = hex.EncodeToString(nonce[:])
		switch event.Method {
		case "GET", "HEAD", "POST", "PUT", "DELETE", "OPTIONS", "PATCH":
		default:
			event.Method = "OTHER"
		}
		deny := func(code int) {
			event.Status = code
			if c.audit != nil {
				if c.audit.Deny(event) != nil {
					w.WriteHeader(503)
					return
				}
			}
			w.WriteHeader(code)
		}
		if !localRequest(r) || strings.Contains(r.URL.Path, "\\") || strings.Contains(r.URL.Path, "..") || r.URL.RawPath != "" {
			deny(403)
			return
		}
		if route == "" {
			w.WriteHeader(404)
			return
		}
		if c.mode == "mtls" {
			policy, e := loadPolicy(c.policyPath)
			if e != nil {
				deny(503)
				return
			}
			client, e := policy.authenticate(r.TLS, event.Time)
			if e != nil {
				deny(401)
				return
			}
			principal.ID = client.ID
			principal.Role = client.Role
			principal.Authenticated = true
			event.PrincipalID = client.ID
			event.Role = client.Role
		}
		if !allowed(principal.Role, route, r.Method) || c.mode != "mtls" && !readMethod(r.Method) {
			deny(403)
			return
		}
		if !readMethod(r.Method) {
			if r.URL.RawQuery != "" {
				deny(400)
				return
			}
			end, e := c.audit.Begin(event)
			if e != nil {
				w.WriteHeader(503)
				return
			}
			writer := &statusWriter{ResponseWriter: w}
			completed := false
			defer func() {
				status := writer.status
				if !completed {
					status = 500
				} else if status == 0 {
					status = 200
				}
				_ = end(status)
			}()
			next.ServeHTTP(writer, r.WithContext(context.WithValue(r.Context(), principalKey{}, principal)))
			completed = true
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey{}, principal)))
	})
}
