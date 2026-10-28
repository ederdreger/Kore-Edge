package live

import (
	"bytes"
	"context"
	"image/jpeg"
	"io"
	"kore-edge/internal/mediaresources"
	"net/http"
	"os/exec"
	"sync"
	"time"
)

const thumbnailTTL = 15 * time.Second
const thumbnailLimit = 128 * 1024

var thumbnailSlots = make(chan struct{}, 2)

type thumbnailEntry struct {
	hub   *Hub
	at    time.Time
	done  chan struct{}
	image []byte
	err   error
}
type thumbnailCache struct {
	mu       sync.Mutex
	entries  map[string]*thumbnailEntry
	generate func(context.Context, string) ([]byte, error)
}
type thumbnailBuffer struct{ bytes.Buffer }

func (b *thumbnailBuffer) Write(data []byte) (int, error) {
	if b.Len()+len(data) > thumbnailLimit {
		return 0, io.ErrShortBuffer
	}
	return b.Buffer.Write(data)
}

func (m *Manager) thumbnailHub(id string) (*Hub, bool) {
	if !m.PlaybackReady(id) {
		return nil, false
	}
	m.mu.Lock()
	j := m.jobs[id]
	closed := m.closed
	m.mu.Unlock()
	if closed || j == nil {
		return nil, false
	}
	j.mu.Lock()
	hub := j.hub
	running := j.status.State == "running"
	j.mu.Unlock()
	return hub, running && m.PlaybackAllowed(id, false)
}

// Thumbnail reads only the processed shared output, never starts an origin.
func (m *Manager) Thumbnail(parent context.Context, id string) ([]byte, error) {
	if !safeID.MatchString(id) {
		return nil, ErrInvalid
	}
	hub, ok := m.thumbnailHub(id)
	if !ok {
		return nil, ErrDenied
	}
	c := &m.thumbnails
	c.mu.Lock()
	if c.entries == nil {
		c.entries = map[string]*thumbnailEntry{}
	}
	entry := c.entries[id]
	if entry != nil && entry.hub == hub && time.Since(entry.at) < thumbnailTTL {
		c.mu.Unlock()
		select {
		case <-parent.Done():
			return nil, parent.Err()
		case <-entry.done:
		}
		if current, ok := m.thumbnailHub(id); !ok || current != hub {
			return nil, ErrDenied
		}
		return entry.image, entry.err
	}
	select {
	case thumbnailSlots <- struct{}{}:
	default:
		c.mu.Unlock()
		return nil, mediaresources.ErrBusy
	}
	if len(c.entries) >= 128 {
		var oldest string
		var at time.Time
		for key, value := range c.entries {
			select {
			case <-value.done:
				if oldest == "" || value.at.Before(at) {
					oldest, at = key, value.at
				}
			default:
			}
		}
		if oldest != "" {
			delete(c.entries, oldest)
		} else {
			<-thumbnailSlots
			c.mu.Unlock()
			return nil, mediaresources.ErrBusy
		}
	}
	entry = &thumbnailEntry{hub: hub, at: time.Now(), done: make(chan struct{})}
	c.entries[id] = entry
	c.mu.Unlock()
	defer func() { <-thumbnailSlots; close(entry.done) }()
	ctx, cancel := context.WithTimeout(parent, 4*time.Second)
	defer cancel()
	stop := context.AfterFunc(m.ctx, cancel)
	defer stop()
	if m.thumbnails.generate != nil {
		entry.image, entry.err = m.thumbnails.generate(ctx, id)
	} else {
		entry.image, entry.err = m.renderThumbnail(ctx, id)
	}
	if current, ok := m.thumbnailHub(id); !ok || current != hub {
		entry.image, entry.err = nil, ErrDenied
	}
	return entry.image, entry.err
}

func (m *Manager) renderThumbnail(ctx context.Context, id string) ([]byte, error) {
	tool := m.store.snapshot().Tools.FFmpeg
	if tool == "" {
		return nil, ErrUnavailable
	}
	input, err := m.SharedStream(ctx, id)
	if err != nil {
		return nil, err
	}
	defer input.Close()
	cmd := exec.CommandContext(ctx, tool, "-hide_banner", "-loglevel", "quiet", "-nostdin", "-filter_threads", "1", "-protocol_whitelist", "pipe", "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-frames:v", "1", "-vf", "scale=480:270:force_original_aspect_ratio=decrease,pad=480:270:(ow-iw)/2:(oh-ih)/2", "-an", "-c:v", "mjpeg", "-threads:v", "1", "-q:v", "5", "-f", "image2pipe", "pipe:1")
	mediaresources.Configure(cmd)
	cmd.WaitDelay = time.Second
	out := &thumbnailBuffer{}
	cmd.Stdout = out
	cmd.Stderr = io.Discard
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, ErrUnavailable
	}
	if cmd.Start() != nil {
		stdin.Close()
		return nil, ErrUnavailable
	}
	done := make(chan struct{})
	go func() { defer close(done); io.Copy(stdin, io.LimitReader(input, 4*1024*1024)); stdin.Close() }()
	err = cmd.Wait()
	input.Close()
	stdin.Close()
	<-done
	if err != nil || ctx.Err() != nil {
		return nil, ErrUnavailable
	}
	config, err := jpeg.DecodeConfig(bytes.NewReader(out.Bytes()))
	if err != nil || config.Width != 480 || config.Height != 270 {
		return nil, ErrUnavailable
	}
	return out.Bytes(), nil
}

func (m *Manager) ServeThumbnail(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Cache-Control", "no-store")
	data, err := m.Thumbnail(r.Context(), id)
	if err != nil {
		w.Header().Set("Retry-After", "15")
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(data)
}
