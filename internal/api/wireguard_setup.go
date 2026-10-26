package api

import (
	"encoding/json"
	"errors"
	"io"
	"kore-edge/internal/accounts"
	"kore-edge/internal/catalog"
	"kore-edge/internal/wgbridge"
	"kore-edge/internal/wireguard"
	"net/http"
	"strings"
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
	if strings.HasPrefix(r.URL.Path, "/api/v1/wireguard/setup/connection/") {
		a.wireGuardConnection(w, r, u)
		return
	}
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

func (a *API) wireGuardConnection(w http.ResponseWriter, r *http.Request, u accounts.User) {
	socket := a.WireGuardBridgeSocket
	if socket == "" {
		socket = wgbridge.EdgeSocket
	}
	var output any
	var err error
	switch {
	case r.Method == "POST" && r.URL.Path == "/api/v1/wireguard/setup/connection/claim":
		var input struct {
			CurrentPassword string                    `json:"current_password"`
			Confirmed       bool                      `json:"confirmed"`
			Invitation      string                    `json:"invitation"`
			Revision        uint64                    `json:"revision"`
			Local           wireguard.LocalConnection `json:"local"`
		}
		if !setupBody(r, &input) || !input.Confirmed {
			w.WriteHeader(400)
			return
		}
		var state wireguard.CredentialStatus
		err = a.Accounts.WireGuardChange(u, input.CurrentPassword, "wireguard_pairing", func() error {
			var e error
			state, e = a.WireGuardCredentials.ClaimConnection(r.Context(), input.Invitation, input.Local, input.Revision)
			return e
		})
		output = state
	case r.Method == "GET" && r.URL.Path == "/api/v1/wireguard/setup/connection/plan":
		var plan wgbridge.Plan
		err = wgbridge.Call(r.Context(), socket, "GET", "/plan", nil, &plan)
		output = plan
	case r.Method == "GET" && r.URL.Path == "/api/v1/wireguard/setup/connection/status":
		var state wgbridge.Status
		err = wgbridge.Call(r.Context(), socket, "GET", "/status", nil, &state)
		output = state
	case r.Method == "POST" && (r.URL.Path == "/api/v1/wireguard/setup/connection/apply" || r.URL.Path == "/api/v1/wireguard/setup/connection/rollback"):
		var input struct {
			CurrentPassword string `json:"current_password"`
			Confirmed       bool   `json:"confirmed"`
			Digest          string `json:"digest"`
		}
		if !setupBody(r, &input) || !input.Confirmed {
			w.WriteHeader(400)
			return
		}
		path := "/apply"
		if strings.HasSuffix(r.URL.Path, "/rollback") {
			path = "/rollback"
		}
		var state wgbridge.Status
		err = a.Accounts.WireGuardChange(u, input.CurrentPassword, "wireguard_connection", func() error {
			return wgbridge.Call(r.Context(), socket, "POST", path, map[string]any{"confirmed": true, "digest": input.Digest}, &state)
		})
		output = state
	default:
		w.WriteHeader(405)
		return
	}
	if err != nil {
		code := 503
		if errors.Is(err, accounts.ErrRejected) {
			code = 403
		}
		if errors.Is(err, wireguard.ErrPeerSettings) {
			code = 400
		}
		if errors.Is(err, wgbridge.ErrConflict) || errors.Is(err, wireguard.ErrRevision) {
			code = 409
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "Conexao nao aplicada. Confira o plano e habilite o auxiliar WireGuard revisado neste servidor."})
		return
	}
	_ = json.NewEncoder(w).Encode(output)
}
func setupBody(r *http.Request, out any) bool {
	b, e := io.ReadAll(io.LimitReader(r.Body, 8193))
	return e == nil && len(b) <= 8192 && catalog.DecodeStrict(b, out) == nil
}
