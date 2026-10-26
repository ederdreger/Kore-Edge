package api

import (
	"encoding/json"
	"errors"
	"io"
	"kore-edge/internal/accounts"
	"kore-edge/internal/catalog"
	"kore-edge/internal/wireguard"
	"net/http"
)

func (a *API) wireGuardSetup(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	u, ok := accounts.UserFromContext(r.Context())
	if !ok || u.Role != "admin" {
		w.WriteHeader(403)
		return
	}
	if a.WireGuardCredentials == nil {
		w.WriteHeader(503)
		return
	}
	var status wireguard.CredentialStatus
	var e error
	if r.URL.Path == "/api/v1/wireguard/setup" && r.Method == "GET" {
		status, e = a.WireGuardCredentials.Snapshot()
	} else if r.URL.Path == "/api/v1/wireguard/setup/identity" && r.Method == "POST" {
		var input struct {
			CurrentPassword string `json:"current_password"`
			Confirmed       bool   `json:"confirmed"`
		}
		if !setupBody(r, &input) || !input.Confirmed {
			w.WriteHeader(400)
			return
		}
		e = a.Accounts.WireGuardChange(u, input.CurrentPassword, "wireguard_identity", func() error { status, e = a.WireGuardCredentials.EnsureIdentity(); return e })
	} else if r.URL.Path == "/api/v1/wireguard/setup/peer" && r.Method == "PUT" {
		var input struct {
			CurrentPassword string                 `json:"current_password"`
			Confirmed       bool                   `json:"confirmed"`
			Revision        uint64                 `json:"revision"`
			Settings        wireguard.PeerSettings `json:"settings"`
		}
		if !setupBody(r, &input) || !input.Confirmed || input.Settings.Validate() != nil {
			w.WriteHeader(400)
			return
		}
		e = a.Accounts.WireGuardChange(u, input.CurrentPassword, "wireguard_peer", func() error { status, e = a.WireGuardCredentials.SavePeer(input.Settings, input.Revision); return e })
	} else {
		w.WriteHeader(405)
		return
	}
	if e != nil {
		code := 503
		if errors.Is(e, accounts.ErrRejected) {
			code = 403
		} else if errors.Is(e, wireguard.ErrPeerSettings) {
			code = 400
		} else if errors.Is(e, wireguard.ErrRevision) {
			code = 409
		}
		w.WriteHeader(code)
		return
	}
	_ = json.NewEncoder(w).Encode(status)
}
func setupBody(r *http.Request, out any) bool {
	b, e := io.ReadAll(io.LimitReader(r.Body, 8193))
	return e == nil && len(b) <= 8192 && catalog.DecodeStrict(b, out) == nil
}
