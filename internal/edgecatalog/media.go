package edgecatalog

import (
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

type Probe struct {
	Protocol    string    `json:"protocol"`
	Reachable   bool      `json:"reachable"`
	Format      string    `json:"format,omitempty"`
	SampleBytes int       `json:"sample_bytes,omitempty"`
	CheckedAt   time.Time `json:"checked_at"`
	Quality     string    `json:"quality"`
	Detail      string    `json:"detail"`
}

func (s *Service) Probe(ctx context.Context, id, protocol string) (Probe, error) {
	select {
	case s.probeSlots <- struct{}{}:
		defer func() { <-s.probeSlots }()
	default:
		return Probe{}, ErrConflict
	}
	if !safeID.MatchString(id) || !s.allowed(id, protocol) {
		return Probe{}, ErrDenied
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var result Probe
	e := s.signed(ctx, "/api/kore-edge/probe/"+id+"/"+protocol, &result)
	if e != nil {
		return result, e
	}
	if result.Protocol != protocol || result.CheckedAt.After(time.Now().Add(30*time.Second)) || result.CheckedAt.Before(time.Now().Add(-time.Minute)) {
		return Probe{}, ErrDenied
	}
	return result, nil
}
func (s *Service) Media(w http.ResponseWriter, r *http.Request, id, protocol, tail string) {
	select {
	case s.mediaSlots <- struct{}{}:
		defer func() { <-s.mediaSlots }()
	default:
		w.WriteHeader(429)
		return
	}
	if !s.allowed(id, protocol) {
		w.WriteHeader(403)
		return
	}
	path, e := mediaPath(id, protocol, tail)
	if e != nil {
		w.WriteHeader(400)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), time.Hour)
	defer cancel()
	// Stop an existing local relay when its lease expires or configuration changes.
	done := make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !s.allowed(id, protocol) {
					cancel()
					return
				}
			}
		}
	}()
	defer func() { cancel(); <-done }()
	response, _, e := s.remote(ctx, path, r.Header.Get("Range"))
	if e != nil {
		if e == ErrDenied {
			w.WriteHeader(403)
		} else {
			w.WriteHeader(502)
		}
		return
	}
	defer response.Body.Close()
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Accel-Buffering", "no")
	mime := response.Header.Get("Content-Type")
	control := http.NewResponseController(w)
	if strings.Contains(strings.ToLower(mime), "mpegurl") {
		body, e := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
		if e != nil || len(body) > 1024*1024 || !strings.HasPrefix(string(body), "#EXTM3U") {
			w.WriteHeader(502)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		text := strings.ReplaceAll(string(body), "/api/kore-edge/media/", "/api/v1/kore-play/media/")
		_ = control.SetWriteDeadline(time.Now().Add(10 * time.Second))
		_, _ = io.WriteString(w, text)
		return
	}
	w.Header().Set("Content-Type", mime)
	if contentRange := response.Header.Get("Content-Range"); contentRange != "" {
		w.Header().Set("Content-Range", contentRange)
	}
	if protocol == "http_ts" {
		w.Header().Set("Content-Type", "video/mp2t")
	}
	w.WriteHeader(response.StatusCode)
	buffer := make([]byte, 64*1024)
	for {
		n, e := response.Body.Read(buffer)
		if n > 0 {
			if control.SetWriteDeadline(time.Now().Add(10*time.Second)) != nil {
				return
			}
			if _, err := w.Write(buffer[:n]); err != nil {
				return
			}
			_ = control.Flush()
		}
		if e != nil {
			return
		}
	}
}
