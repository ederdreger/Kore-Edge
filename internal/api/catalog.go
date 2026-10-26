package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"kore-edge/internal/catalog"
)

func (a *API) catalogHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodPut {
		w.Header().Set("Allow", "GET, HEAD, PUT")
		w.WriteHeader(405)
		return
	}
	if a.Catalog == nil {
		w.WriteHeader(503)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(`{"error":"catalog disabled"}`))
		}
		return
	}
	if r.Method == http.MethodPut {
		if a.Integration != nil {
			w.WriteHeader(http.StatusConflict)
			_, _ = w.Write([]byte(`{"error":"catalog managed by integration"}`))
			return
		}
		body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, catalog.MaxBytes))
		if e != nil || a.Catalog.Apply(body, time.Now().UTC()) != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"catalog rejected"}`))
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	m, e := a.Catalog.Snapshot(time.Now().UTC())
	if e != nil {
		w.WriteHeader(503)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(`{"error":"catalog unavailable"}`))
		}
		return
	}
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(m)
	}
}
