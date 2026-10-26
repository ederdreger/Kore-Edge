package api

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"kore-edge/internal/catalog"
	"kore-edge/internal/stream"
)

func (a *API) streamHandler(w http.ResponseWriter, r *http.Request) {
	collection := r.URL.Path == "/api/v1/streams"
	allowed := "DELETE"
	if collection {
		allowed = "GET, HEAD, POST"
	}
	if (collection && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPost) || (!collection && r.Method != http.MethodDelete) {
		w.Header().Set("Allow", allowed)
		w.WriteHeader(405)
		return
	}
	if a.Streams == nil {
		w.WriteHeader(503)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(`{"error":"stream engine disabled"}`))
		}
		return
	}
	if !collection {
		id := strings.TrimPrefix(r.URL.Path, "/api/v1/streams/")
		if id == "" || strings.Contains(id, "/") || !a.Streams.Stop(id) {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(202)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(a.Streams.Snapshot())
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1024))
	var request struct {
		ChannelID string `json:"channel_id"`
	}
	if err != nil || catalog.DecodeStrict(body, &request) != nil {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid stream request"}`))
		return
	}
	if err = a.Streams.Start(request.ChannelID); err != nil {
		code := 503
		if errors.Is(err, stream.ErrDenied) {
			code = 403
		}
		if errors.Is(err, stream.ErrLimit) {
			code = 409
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(202)
	_, _ = w.Write([]byte(`{"status":"accepted"}`))
}
