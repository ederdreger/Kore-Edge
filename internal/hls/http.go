package hls

import (
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(405)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/hls/"), "/")
	if len(parts) != 2 || !idPattern.MatchString(parts[0]) || (parts[1] != "index.m3u8" && !segmentPattern.MatchString(parts[1])) {
		w.WriteHeader(404)
		return
	}
	if !m.allow(parts[0], time.Now().UTC()) {
		w.WriteHeader(403)
		return
	}
	select {
	case m.requests <- struct{}{}:
		defer func() { <-m.requests }()
	default:
		w.WriteHeader(429)
		return
	}
	m.mu.Lock()
	s := m.sessions[parts[0]]
	if s == nil {
		m.mu.Unlock()
		w.WriteHeader(404)
		return
	}
	s.mu.Lock()
	bad := s.failed || s.closed
	s.mu.Unlock()
	if bad {
		m.mu.Unlock()
		w.WriteHeader(404)
		return
	}
	name := filepath.Join(s.dir, parts[1])
	info, e := os.Lstat(name)
	if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > MaxSegmentBytes {
		m.mu.Unlock()
		w.WriteHeader(404)
		return
	}
	f, e := os.Open(name)
	if e != nil {
		m.mu.Unlock()
		w.WriteHeader(404)
		return
	}
	data, e := io.ReadAll(io.LimitReader(f, MaxSegmentBytes+1))
	f.Close()
	m.mu.Unlock()
	if e != nil || len(data) > MaxSegmentBytes {
		w.WriteHeader(404)
		return
	}
	if parts[1] == "index.m3u8" {
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	} else {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	http.ServeContent(w, r, parts[1], info.ModTime(), bytes.NewReader(data))
}
