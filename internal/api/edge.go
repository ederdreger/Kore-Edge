package api

import (
	"encoding/json"
	"kore-edge/internal/provision"
	"net/http"
	"runtime"
	"time"
)

func (a *API) edgeHandler(w http.ResponseWriter, r *http.Request, started time.Time) {
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
	enrollment := "disabled"
	var identity *provision.Identity
	if a.Provision != nil {
		status, e := a.Provision.Status(r.Context(), a.ProvisionEdgeID)
		if e == nil {
			enrollment = status.State
			identity = status.Identity
		} else {
			enrollment = "unavailable"
		}
	}
	_ = json.NewEncoder(w).Encode(struct {
		Enrollment string              `json:"enrollment_state"`
		Identity   *provision.Identity `json:"identity"`
		Version    string              `json:"version"`
		Mode       string              `json:"mode"`
		OS         string              `json:"os"`
		Arch       string              `json:"arch"`
		Started    time.Time           `json:"started_at"`
		Uptime     float64             `json:"uptime_seconds"`
		Features   map[string]bool     `json:"features"`
	}{enrollment, identity, "dev", a.mode(), runtime.GOOS, runtime.GOARCH, started, time.Since(started).Seconds(), map[string]bool{"catalog": a.Catalog != nil, "stream": a.Streams != nil, "hls": a.HLS != nil, "provisioning": a.Provision != nil}})
}
