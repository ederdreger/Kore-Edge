package live

import (
	"bufio"
	"net/http"
	"strings"
	"time"
)

func (m *Manager) ServeTS(w http.ResponseWriter, r *http.Request, id string) {
	if !safeID.MatchString(id) {
		w.WriteHeader(400)
		return
	}
	m.mu.Lock()
	j := m.jobs[id]
	m.mu.Unlock()
	if j == nil {
		w.WriteHeader(503)
		return
	}
	if !m.MediaIfActive(w, r, id, j.config.Protocol) {
		w.WriteHeader(503)
	}
}

// MediaIfActive serves the same running ingest used by the configured outputs.
// False means no continuous ingest was requested; callers may offer a clearly
// labelled on-demand preview for backwards compatibility.
func (m *Manager) MediaIfActive(w http.ResponseWriter, r *http.Request, id, protocol string) bool {
	m.mu.Lock()
	j := m.jobs[id]
	m.mu.Unlock()
	if j == nil {
		return false
	}
	j.mu.Lock()
	state := j.status.State
	hub := j.hub
	selected := j.config.Protocol
	j.mu.Unlock()
	if state == "stopped" {
		return false
	}
	if protocol != selected {
		return false
	}
	if !m.catalog.Authorized(id, protocol) {
		w.WriteHeader(403)
		return true
	}
	if state != "running" || time.Since(hub.Snapshot().LastPacket) > 3*time.Second {
		w.Header().Set("Retry-After", "1")
		w.WriteHeader(503)
		return true
	}
	select {
	case m.previewSlots <- struct{}{}:
		defer func() { <-m.previewSlots }()
	default:
		w.WriteHeader(429)
		return true
	}
	replay, stream, release, e := hub.Subscribe()
	if e != nil {
		w.WriteHeader(503)
		return true
	}
	defer release()
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Kore-Edge-Mode", "continuous")
	control := http.NewResponseController(w)
	buffer := bufio.NewWriterSize(w, 64*1024)
	if control.SetWriteDeadline(time.Now().Add(8*time.Second)) != nil {
		return true
	}
	for _, chunk := range replay {
		if j.ctx.Err() != nil || r.Context().Err() != nil {
			return true
		}
		if _, e = buffer.Write(chunk); e != nil {
			return true
		}
	}
	if buffer.Flush() != nil || control.Flush() != nil {
		return true
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return true
		case <-j.ctx.Done():
			return true
		case <-ticker.C:
			if buffer.Buffered() > 0 {
				if control.SetWriteDeadline(time.Now().Add(8*time.Second)) != nil || buffer.Flush() != nil || control.Flush() != nil {
					return true
				}
			}
			if !m.catalog.Authorized(id, protocol) {
				return true
			}
		case chunk, ok := <-stream:
			if !ok {
				return true
			}
			if control.SetWriteDeadline(time.Now().Add(8*time.Second)) != nil {
				return true
			}
			if _, e = buffer.Write(chunk); e != nil {
				return true
			}
			if buffer.Buffered() >= 32*1024 {
				if buffer.Flush() != nil || control.Flush() != nil {
					return true
				}
			}
		}
	}
}
func (m *Manager) ServeHLS(w http.ResponseWriter, r *http.Request, id, tail string) {
	if !safeID.MatchString(id) || !m.catalog.OutputAuthorized(id, "hls") {
		w.WriteHeader(403)
		return
	}
	m.mu.Lock()
	j := m.jobs[id]
	m.mu.Unlock()
	if j == nil {
		w.WriteHeader(503)
		return
	}
	j.mu.Lock()
	segmenter := j.hlsManager
	j.mu.Unlock()
	if segmenter == nil {
		w.WriteHeader(503)
		return
	}
	copy := r.Clone(r.Context())
	url := *r.URL
	copy.URL = &url
	copy.URL.Path = "/hls/" + id + "/" + strings.TrimPrefix(tail, "/")
	segmenter.ServeHTTP(w, copy)
}

// PlaybackAllowed rechecks the current source and output grant on every request.
func (m *Manager) PlaybackAllowed(id string, hls bool) bool {
	for _, c := range m.store.snapshot().Channels {
		if c.ChannelID == id {
			return m.catalog.Authorized(id, c.Protocol) && (!hls || c.HLS && m.catalog.OutputAuthorized(id, "hls"))
		}
	}
	return false
}

func (m *Manager) PlaybackReady(id string) bool {
	m.mu.Lock()
	j := m.jobs[id]
	m.mu.Unlock()
	if j == nil {
		return false
	}
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.status.State == "running" && j.hub != nil && time.Since(j.hub.Snapshot().LastPacket) < 3*time.Second
}
