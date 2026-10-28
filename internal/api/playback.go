package api

import (
	"context"
	"encoding/json"
	"kore-edge/internal/accounts"
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
	var ids []string
	switch r.Method {
	case "GET":
		if r.URL.RawQuery != "" {
			w.WriteHeader(400)
			return
		}
		view := a.Live.Snapshot()
		seen := map[string]bool{}
		for _, c := range view.Configuration.Channels {
			if !seen[c.ChannelID] {
				ids = append(ids, c.ChannelID)
				seen[c.ChannelID] = true
			}
		}
		if view.Catalog.Catalog != nil {
			for _, c := range view.Catalog.Catalog.Channels {
				if !seen[c.ID] {
					ids = append(ids, c.ID)
					seen[c.ID] = true
				}
			}
		}
	case "POST":
		// Compatibility for cached exports. No rotating/revoking playback action.
		var input struct {
			Action   string   `json:"action"`
			Channels []string `json:"channel_ids"`
		}
		if !setupBody(r, &input) || input.Action != "issue" || len(input.Channels) == 0 || len(input.Channels) > 128 {
			w.WriteHeader(400)
			return
		}
		known := map[string]bool{}
		v := a.Live.Snapshot()
		for _, c := range v.Configuration.Channels {
			known[c.ChannelID] = true
		}
		if v.Catalog.Catalog != nil {
			for _, c := range v.Catalog.Catalog.Channels {
				known[c.ID] = true
			}
		}
		seen := map[string]bool{}
		for _, id := range input.Channels {
			if !known[id] || seen[id] {
				w.WriteHeader(403)
				return
			}
			seen[id] = true
		}
		ids = input.Channels
	default:
		w.WriteHeader(405)
		return
	}
	type link struct {
		ID    string `json:"channel_id"`
		TS    string `json:"ts_path"`
		HLS   string `json:"hls_path"`
		Embed string `json:"embed_path"`
		DASH  string `json:"dash_path"`
		CMAF  string `json:"cmaf_path"`
		Info  string `json:"info_path"`
	}
	links := []link{}
	for _, id := range ids {
		raw, e := a.Playback.Permanent(id)
		if e != nil {
			w.WriteHeader(503)
			return
		}
		base := "/api/v1/playback/" + raw + "/" + id + "/"
		links = append(links, link{ID: id, TS: base + "stream.ts", HLS: base + "index.m3u8", Embed: base + "embed.html", DASH: base + "cmaf/index.mpd", CMAF: base + "cmaf/master.m3u8", Info: base + "media_info.json"})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"links": links})
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
	embedPage := strings.HasSuffix(r.URL.Path, "/embed.html") || strings.HasSuffix(r.URL.Path, "/embed.js") || strings.HasSuffix(r.URL.Path, "/hls.js")
	if r.Header.Get("Sec-Fetch-Site") == "cross-site" && !embedPage || r.URL.RawPath != "" || r.URL.RawQuery != "" || strings.ContainsAny(r.URL.Path, "\\") || strings.Contains(r.URL.Path, "..") {
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
	if len(parts) != 3 && !(len(parts) == 4 && parts[2] == "cmaf") {
		w.WriteHeader(404)
		return
	}
	raw, id, tail := parts[0], parts[1], parts[2]
	if embedPage && (len(parts) != 3 || (tail != "embed.html" && tail != "embed.js" && tail != "hls.js")) {
		w.WriteHeader(404)
		return
	}
	segmented := len(parts) == 4
	isHLS := tail == "index.m3u8" || playbackSegment.MatchString(tail) || segmented || embedPage
	if !embedPage && !segmented && tail != "media_info.json" && tail != "stream.ts" && tail != "index.m3u8" && !playbackSegment.MatchString(tail) {
		w.WriteHeader(404)
		return
	}
	allowed := func() bool {
		c, e := a.Playback.Verify(raw, id, isHLS)
		return e == nil && (c.Permanent || a.Accounts.PlaybackOwner(c.Owner, c.Revision)) && a.Live.PlaybackAllowed(id, isHLS)
	}
	if !allowed() {
		w.WriteHeader(401)
		return
	}
	if embedPage {
		serveEmbed(w, r, tail)
		return
	}
	if tail == "media_info.json" {
		w.Header().Set("Content-Type", "application/json")
		v := a.Live.Snapshot()
		for _, status := range v.Streams {
			if status.ChannelID == id {
				if r.Method != "HEAD" {
					info := map[string]any{"channel_id": id, "state": status.State, "bits_per_second": status.Metrics.Bitrate, "last_packet_at": status.Metrics.LastPacket, "interruptions": status.Interruptions, "hls_state": status.HLS, "udp_state": status.UDP}
					if a.ManualChannels != nil && status.State == "running" {
						if media, err := a.ManualChannels.Processing(id); err == nil {
							// Explicit allowlist: never serialize source addresses or credentials.
							info["input_media"], info["output_media"], info["input_srt"] = media.InputMedia, media.OutputMedia, media.InputSRT
							for _, input := range media.Inputs {
								if input.Key == media.ActiveKey {
									info["input_protocol"] = input.Protocol
									break
								}
							}
						}
					}
					json.NewEncoder(w).Encode(info)
				}
				return
			}
		}
		w.WriteHeader(503)
		return
	}
	if segmented {
		if a.Outputs == nil {
			w.WriteHeader(503)
			return
		}
		a.Outputs.ServeSegment(w, r, id, parts[3])
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
	c, _ := a.Playback.Verify(raw, id, false)
	if c.Permanent {
		a.Live.ServePersistentTS(w, r.WithContext(ctx), id)
	} else {
		a.Live.ServeTS(w, r.WithContext(ctx), id)
	}
}
