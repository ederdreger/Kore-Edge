package api

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/multicast"
)

func (a *API) multicastHandler(w http.ResponseWriter, r *http.Request) {
	plan := r.URL.Path == "/api/v1/multicast/plan"
	if (plan && r.Method != http.MethodPost) || (!plan && r.Method != http.MethodGet && r.Method != http.MethodHead) {
		allowed := "GET, HEAD"
		if plan {
			allowed = "POST"
		}
		w.Header().Set("Allow", allowed)
		w.WriteHeader(405)
		return
	}
	var target multicast.Target
	if plan {
		b, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 2048))
		if e != nil || catalog.DecodeStrict(b, &target) != nil || target.Validate() != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(`{"error":"invalid multicast target"}`))
			return
		}
	}
	lookup := a.MulticastInterfaces
	if lookup == nil {
		lookup = multicast.Interfaces
	}
	all, e := lookup()
	if e != nil {
		w.WriteHeader(503)
		if r.Method != http.MethodHead {
			_, _ = w.Write([]byte(`{"error":"interface inventory unavailable"}`))
		}
		return
	}
	if plan {
		p, e := multicast.DryRun(target, all)
		if e != nil {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": e.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(p)
		return
	}
	if r.Method == http.MethodGet {
		_ = json.NewEncoder(w).Encode(struct {
			Timestamp  time.Time             `json:"timestamp"`
			State      string                `json:"state"`
			Interfaces []multicast.Interface `json:"interfaces"`
		}{time.Now().UTC(), "real", all})
	}
}
