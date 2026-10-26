package wgbridge

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"time"
)

const EdgeSocket = "/run/kore-wg-bridge-edge/control.sock"

func Call(ctx context.Context, socket, method, path string, input, output any) error {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", socket)
	}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 4 * time.Second}
	b, err := json.Marshal(input)
	if err != nil {
		return ErrUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, method, "http://local"+path, bytes.NewReader(b))
	if err != nil {
		return ErrUnavailable
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return ErrUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		if response.StatusCode == 409 {
			return ErrConflict
		}
		return ErrUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 256*1024))
	if err != nil || json.Unmarshal(body, output) != nil {
		return ErrUnavailable
	}
	return nil
}
func Handler(engine *Engine) http.Handler {
	slots := make(chan struct{}, 8)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			w.WriteHeader(429)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		var value any
		var err error
		switch {
		case r.Method == "GET" && r.URL.Path == "/plan":
			value, err = engine.Plan(ctx)
		case r.Method == "GET" && r.URL.Path == "/status":
			value = engine.Status(ctx)
		case r.Method == "POST" && (r.URL.Path == "/apply" || r.URL.Path == "/rollback"):
			var input struct {
				Digest    string `json:"digest"`
				Confirmed bool   `json:"confirmed"`
			}
			decoder := json.NewDecoder(io.LimitReader(r.Body, 2048))
			decoder.DisallowUnknownFields()
			if decoder.Decode(&input) != nil || !input.Confirmed {
				w.WriteHeader(400)
				return
			}
			var extra any
			if decoder.Decode(&extra) != io.EOF {
				w.WriteHeader(400)
				return
			}
			if r.URL.Path == "/apply" {
				value, err = engine.Apply(input.Digest)
			} else {
				value, err = engine.Rollback()
			}
		default:
			w.WriteHeader(404)
			return
		}
		if err != nil {
			code := 503
			if err == ErrConflict {
				code = 409
			}
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(value)
	})
}
