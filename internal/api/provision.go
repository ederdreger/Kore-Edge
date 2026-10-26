package api

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"encoding/json"
	"kore-edge/internal/catalog"
	"kore-edge/internal/provision"
)

func (a *API) provisionHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	register := r.URL.Path == "/api/v1/provisioning/register"
	if register && r.Method != http.MethodPost || !register && r.Method != http.MethodGet && r.Method != http.MethodHead {
		allowed := "GET, HEAD"
		if register {
			allowed = "POST"
		}
		w.Header().Set("Allow", allowed)
		w.WriteHeader(405)
		return
	}
	if a.Provision == nil {
		w.WriteHeader(503)
		return
	}
	if r.URL.RawQuery != "" {
		w.WriteHeader(400)
		return
	} // No bearer/identity in URLs.
	if !register {
		status, e := a.Provision.Status(r.Context(), a.ProvisionEdgeID)
		if e != nil {
			w.WriteHeader(503)
			return
		}
		if r.Method != http.MethodHead {
			_ = json.NewEncoder(w).Encode(status)
		}
		return
	}
	authorization := r.Header.Get("Authorization")
	if len(authorization) != len("Bearer ")+47 || !strings.HasPrefix(authorization, "Bearer ") {
		w.WriteHeader(403)
		_, _ = w.Write([]byte(`{"error":"enrollment rejected"}`))
		return
	}
	body, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	var request provision.Request
	if e != nil || catalog.DecodeStrict(body, &request) != nil {
		w.WriteHeader(400)
		_, _ = w.Write([]byte(`{"error":"invalid enrollment request"}`))
		return
	}
	// Exact property names prevent case-folded duplicate aliases in encoding/json.
	var fields map[string]json.RawMessage
	if json.Unmarshal(body, &fields) != nil {
		w.WriteHeader(400)
		return
	}
	for key := range fields {
		if key != "version" && key != "edge_id" && key != "public_key" && key != "challenge" && key != "signature" {
			w.WriteHeader(400)
			return
		}
	}
	identity, e := a.Provision.Register(r.Context(), a.ProvisionEdgeID, strings.TrimPrefix(authorization, "Bearer "), request, time.Now().UTC())
	if e != nil {
		code := 403
		if errors.Is(e, provision.ErrStorage) || errors.Is(e, provision.ErrBusy) {
			code = 503
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(`{"error":"enrollment rejected"}`))
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(provision.Status{State: "registered", Identity: &identity, Applied: false, PermissionsVerified: a.Provision.PermissionsVerified()})
}
