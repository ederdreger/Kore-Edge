package api

import (
	"encoding/json"
	"net/http"
	"time"

	"kore-edge/internal/monitoring"
)

func (a *API) monitoringHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	if r.URL.Path == "/metrics" {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	} else {
		w.Header().Set("Content-Type", "application/json")
	}
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(405)
		return
	}
	if a.Monitoring == nil {
		w.WriteHeader(503)
		return
	}
	snapshot := a.Monitoring.Snapshot(time.Now().UTC())
	if r.Method == http.MethodHead {
		return
	}
	if r.URL.Path == "/metrics" {
		monitoring.Export(w, snapshot)
	} else {
		_ = json.NewEncoder(w).Encode(snapshot)
	}
}
