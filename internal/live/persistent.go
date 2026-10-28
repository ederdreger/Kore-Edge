package live

import (
	"bufio"
	"net/http"
	"time"
)

// persistentJob follows a reconfigured job, but never starts a source or ignores
// an explicit pause. Callers validate the playback capability independently.
func (m *Manager) persistentJob(id string) (*job, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || m.ctx.Err() != nil {
		return nil, false
	}
	for _, c := range m.store.snapshot().Channels {
		if c.ChannelID == id {
			if c.AutoPaused {
				return nil, false
			}
			j := m.jobs[id]
			return j, j != nil
		}
	}
	return nil, false
}

// ServePersistentTS retains the HTTP connection across ingest retries/hub
// replacement. Null TS packets keep an idle transport alive without entering
// the source hub, bitrate or quality metrics. They do not simulate video.
func (m *Manager) ServePersistentTS(w http.ResponseWriter, r *http.Request, id string) {
	if !safeID.MatchString(id) {
		w.WriteHeader(400)
		return
	}
	if _, ok := m.persistentJob(id); !ok || !m.PlaybackReady(id) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(503)
		return
	}
	select {
	case m.previewSlots <- struct{}{}:
		defer func() { <-m.previewSlots }()
	default:
		w.WriteHeader(429)
		return
	}
	w, finish := m.observeDelivery(w, id, true)
	defer finish()
	w.Header().Set("Content-Type", "video/mp2t")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.Header().Set("X-Kore-Edge-Mode", "continuous")
	control := http.NewResponseController(w)
	buffer := bufio.NewWriterSize(w, 64*1024)
	var hub *Hub
	var stream <-chan []byte
	var release func()
	defer func() {
		if release != nil {
			release()
		}
	}()
	lastWrite := time.Now()
	write := func(data []byte) bool {
		if r.Context().Err() != nil || control.SetWriteDeadline(time.Now().Add(8*time.Second)) != nil {
			return false
		}
		if _, e := buffer.Write(data); e != nil {
			return false
		}
		lastWrite = time.Now()
		return true
	}
	flush := func() bool {
		return control.SetWriteDeadline(time.Now().Add(8*time.Second)) == nil && buffer.Flush() == nil && control.Flush() == nil
	}
	attach := func() bool {
		j, ok := m.persistentJob(id)
		if !ok {
			return false
		}
		j.mu.Lock()
		next := j.hub
		j.mu.Unlock()
		if next == hub && stream != nil {
			return true
		}
		if release != nil {
			release()
			release = nil
		}
		hub = nil
		stream = nil
		if next == nil {
			return true
		}
		replay, sub, done, e := next.Subscribe()
		if e != nil {
			return true
		}
		hub = next
		stream = sub
		release = done
		for _, chunk := range replay {
			if !write(chunk) {
				return false
			}
		}
		return true
	}
	if !attach() || !flush() {
		return
	}
	null := make([]byte, chunkBytes)
	for i := 0; i < len(null); i += 188 {
		for n := 0; n < 188; n++ {
			null[i+n] = 0xff
		}
		null[i] = 0x47
		null[i+1] = 0x1f
		null[i+2] = 0xff
		null[i+3] = 0x10
	}
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !attach() {
				return
			}
			if time.Since(lastWrite) >= time.Second {
				if !write(null) {
					return
				}
			}
			if buffer.Buffered() > 0 && !flush() {
				return
			}
		case chunk, ok := <-stream:
			if !ok {
				if release != nil {
					release()
					release = nil
				}
				hub = nil
				stream = nil
				continue
			}
			if !write(chunk) {
				return
			}
			if buffer.Buffered() >= 32*1024 && !flush() {
				return
			}
		}
	}
}
