package hls

import (
	"embed"
	"net/http"
	"strings"
	"time"
)

//go:embed web/*
var playerFiles embed.FS

func (m *Manager) Player(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; media-src 'self' blob:; connect-src 'self'; worker-src blob:")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(405)
		return
	}
	name := ""
	switch r.URL.Path {
	case "/player-assets/hls.min.js":
		name = "hls.min.js"
	case "/player-assets/player.js":
		name = "player.js"
	case "/player-assets/player.css":
		name = "player.css"
	}
	if name == "" {
		id := strings.TrimPrefix(r.URL.Path, "/player/")
		if !idPattern.MatchString(id) {
			w.WriteHeader(404)
			return
		}
		if !m.allow(id, time.Now().UTC()) {
			w.WriteHeader(403)
			return
		}
		name = "player.html"
	}
	data, e := playerFiles.ReadFile("web/" + name)
	if e != nil {
		w.WriteHeader(404)
		return
	}
	mime := "text/html; charset=utf-8"
	if strings.HasSuffix(name, ".js") {
		mime = "text/javascript; charset=utf-8"
	}
	if strings.HasSuffix(name, ".css") {
		mime = "text/css; charset=utf-8"
	}
	w.Header().Set("Content-Type", mime)
	if r.Method == http.MethodGet {
		_, _ = w.Write(data)
	}
}
