package api

import (
	"context"
	"encoding/json"
	"errors"
	"kore-edge/internal/accounts"
	"kore-edge/internal/playback"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var playbackSegment = regexp.MustCompile(`^seg[0-9]{9,20}\.ts$`)

func (a *API) playbackLinks(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	u, ok := accounts.UserFromContext(r.Context())
	if !ok || u.Role != "admin" {
		w.WriteHeader(403)
		return
	}
	if a.Playback == nil || a.Live == nil {
		w.WriteHeader(503)
		return
	}
	var input struct {
		Action   string   `json:"action"`
		Channels []string `json:"channel_ids"`
	}
	if r.Method != "POST" || !setupBody(r, &input) || len(input.Channels) == 0 || len(input.Channels) > 128 || (input.Action != "issue" && input.Action != "revoke") {
		w.WriteHeader(400)
		return
	}
	seen := map[string]bool{}
	for _, id := range input.Channels {
		if seen[id] || input.Action == "issue" && !a.Live.PlaybackAllowed(id, false) {
			w.WriteHeader(403)
			return
		}
		seen[id] = true
	}
	type link struct {
		ID      string    `json:"channel_id"`
		TS      string    `json:"ts_path"`
		HLS     string    `json:"hls_path,omitempty"`
		Expires time.Time `json:"expires_at"`
	}
	links := []link{}
	e := a.Accounts.ChannelChange(u, "playback_"+input.Action, func() error {
		if input.Action == "revoke" {
			return a.Playback.Revoke(input.Channels)
		}
		for _, id := range input.Channels {
			if !a.Live.PlaybackAllowed(id, false) {
				return playback.ErrDenied
			}
			hls := a.Live.PlaybackAllowed(id, true)
			raw, expires, e := a.Playback.Issue(id, u.ID, u.Revision, hls)
			if e != nil {
				return e
			}
			base := "/api/v1/playback/" + raw + "/" + id + "/"
			out := link{ID: id, TS: base + "stream.ts", Expires: expires}
			if hls {
				out.HLS = base + "index.m3u8"
			}
			links = append(links, out)
		}
		return nil
	})
	if e != nil {
		code := 503
		if errors.Is(e, accounts.ErrBusy) {
			code = 429
		}
		if errors.Is(e, accounts.ErrRejected) || errors.Is(e, playback.ErrDenied) {
			code = 403
		}
		w.WriteHeader(code)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"links": links, "revoked": input.Action == "revoke"})
}

func samePlaybackOrigin(raw, wanted string) bool {
	canonical := func(raw string) string {
		u, e := url.Parse(raw)
		if e != nil || u.Scheme != "https" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || u.Hostname() == "" {
			return ""
		}
		port := u.Port()
		if port == "" {
			port = "443"
		}
		return net.JoinHostPort(strings.ToLower(u.Hostname()), port)
	}
	a, b := canonical(raw), canonical(wanted)
	return a != "" && a == b
}

func (a *API) externalPlayback(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// Only the installer-managed HTTPS gateway may reach this backend endpoint.
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() || !samePlaybackOrigin(a.PublicOrigin, a.PublicOrigin) {
		w.WriteHeader(403)
		return
	}
	if origin := r.Header.Get("Origin"); origin != "" && !samePlaybackOrigin(origin, a.PublicOrigin) {
		w.WriteHeader(403)
		return
	}
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" || r.URL.RawPath != "" || r.URL.RawQuery != "" || strings.ContainsAny(r.URL.Path, "\\") || strings.Contains(r.URL.Path, "..") {
		w.WriteHeader(400)
		return
	}
	if r.Method != "GET" && r.Method != "HEAD" {
		w.WriteHeader(405)
		return
	}
	if a.Playback == nil || a.Live == nil || a.Accounts == nil {
		w.WriteHeader(503)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/v1/playback/"), "/")
	if len(parts) != 3 {
		w.WriteHeader(404)
		return
	}
	raw, id, tail := parts[0], parts[1], parts[2]
	isHLS := tail != "stream.ts"
	if isHLS && tail != "index.m3u8" && !playbackSegment.MatchString(tail) {
		w.WriteHeader(404)
		return
	}
	allowed := func() bool {
		c, e := a.Playback.Verify(raw, id, isHLS)
		return e == nil && a.Accounts.PlaybackOwner(c.Owner, c.Revision) && a.Live.PlaybackAllowed(id, isHLS)
	}
	if !allowed() {
		w.WriteHeader(401)
		return
	}
	release, ok := a.Playback.Acquire(raw)
	if !ok {
		w.WriteHeader(429)
		return
	}
	defer release()
	if isHLS {
		a.Live.ServeHLS(w, r, id, tail)
		return
	}
	if r.Method == "HEAD" {
		if !a.Live.PlaybackReady(id) {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "video/mp2t")
		w.WriteHeader(200)
		return
	}
	ctx, cancel := context.WithCancel(r.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !allowed() {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	a.Live.ServeTS(w, r.WithContext(ctx), id)
}
