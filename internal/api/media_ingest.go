package api

import (
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// Machine integration has a separate channel-scoped credential; no administrator password.
func (a *API) mediaIngest(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	ip := net.ParseIP(host)
	if e != nil || ip == nil || !ip.IsLoopback() || a.PublicOrigin == "" || a.ManualChannels == nil || a.Live == nil {
		w.WriteHeader(403)
		return
	}
	if r.Header.Get("Cookie") != "" || r.Header.Get("Origin") != "" || r.URL.RawQuery != "" {
		w.WriteHeader(403)
		return
	}
	publish := strings.HasPrefix(r.URL.Path, "/api/v1/publish/")
	prefix := "/api/v1/cluster/"
	if publish {
		prefix = "/api/v1/publish/"
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "stream.ts" {
		w.WriteHeader(404)
		return
	}
	id := parts[0]
	auth := r.Header.Get("Authorization")
	if !strings.HasPrefix(auth, "Bearer ") || !a.ManualChannels.IngestAuthorized(id, strings.TrimPrefix(auth, "Bearer "), publish) {
		w.WriteHeader(401)
		return
	}
	if !publish {
		if r.Method != "GET" {
			w.WriteHeader(405)
			return
		}
		profile, e := a.ManualChannels.Processing(id)
		if e != nil {
			w.WriteHeader(404)
			return
		}
		// Deliberately one hop: a cluster-sourced channel cannot be exported again.
		for _, in := range profile.Inputs {
			if in.Key == profile.ActiveKey && in.Protocol == "cluster" {
				w.WriteHeader(409)
				return
			}
		}
		a.Live.ServePersistentTS(w, r, id)
		return
	}
	if r.Method != "POST" && r.Method != "PUT" {
		w.WriteHeader(405)
		return
	}
	writer, e := a.ManualChannels.AttachPublisher(id)
	if e != nil {
		w.WriteHeader(409)
		return
	}
	defer writer.Close()
	controller := http.NewResponseController(w)
	controller.EnableFullDuplex()
	controller.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(200)
	controller.Flush()
	stop := make(chan struct{})
	finished := make(chan struct{})
	defer func() { close(stop); <-finished }()
	go func() {
		defer close(finished)
		ticker := time.NewTicker(15 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-r.Context().Done():
				return
			case <-ticker.C:
				controller.SetWriteDeadline(time.Now().Add(5 * time.Second))
				if _, e := w.Write([]byte("\n")); e != nil {
					writer.Close()
					r.Body.Close()
					return
				}
				if controller.Flush() != nil {
					writer.Close()
					r.Body.Close()
					return
				}
				controller.SetWriteDeadline(time.Time{})
			}
		}
	}()
	// Bounded idle deadline, but no lifetime expiry while bytes keep arriving.
	buffer := make([]byte, 188*7)
	for {
		if !a.ManualChannels.IngestAuthorized(id, strings.TrimPrefix(auth, "Bearer "), true) {
			return
		}
		controller.SetReadDeadline(time.Now().Add(20 * time.Second))
		n, e := r.Body.Read(buffer)
		if n > 0 {
			if _, writeErr := writer.Write(buffer[:n]); writeErr != nil {
				return
			}
		}
		if e != nil {
			if e != io.EOF {
				return
			}
			return
		}
	}
}
