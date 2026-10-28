package live

import (
	"context"
	"io"
	"time"
)

// SharedStream is an internal output subscription, never an origin opener.
// It follows recovery, but terminates on explicit stop or authorization loss.
func (m *Manager) SharedStream(parent context.Context, id string) (io.ReadCloser, error) {
	if !safeID.MatchString(id) {
		return nil, ErrInvalid
	}
	if _, ok := m.persistentJob(id); !ok || !m.PlaybackAllowed(id, false) {
		return nil, ErrDenied
	}
	ctx, cancel := context.WithCancel(parent)
	r, w := io.Pipe()
	go func() {
		select {
		case <-ctx.Done():
			r.CloseWithError(ErrUnavailable)
		}
	}()
	go func() {
		defer cancel()
		defer w.Close()
		var hub *Hub
		var stream <-chan []byte
		var release func()
		defer func() {
			if release != nil {
				release()
			}
		}()
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		attach := func() bool {
			j, ok := m.persistentJob(id)
			if !ok || !m.PlaybackAllowed(id, false) {
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
			for _, b := range replay {
				if _, e = w.Write(b); e != nil {
					return false
				}
			}
			return true
		}
		if !attach() {
			return
		}
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !attach() {
					return
				}
			case b, ok := <-stream:
				if !ok {
					if release != nil {
						release()
						release = nil
					}
					hub = nil
					stream = nil
					continue
				}
				if _, e := w.Write(b); e != nil {
					return
				}
			}
		}
	}()
	return &sharedReader{ReadCloser: r, cancel: cancel}, nil
}

type sharedReader struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *sharedReader) Close() error { r.cancel(); return r.ReadCloser.Close() }
