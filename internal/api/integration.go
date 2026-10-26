package api

import (
	"encoding/json"
	"kore-edge/internal/integration"
	"net/http"
	"time"
)

func (a *API) integrationHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		w.WriteHeader(405)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	st := integration.Status{Mode: "disabled", State: "disabled"}
	if a.Integration != nil {
		st = a.Integration.Snapshot(time.Now().UTC())
	}
	_ = json.NewEncoder(w).Encode(st)
}
