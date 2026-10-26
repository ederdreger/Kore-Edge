package api

import (
	"encoding/json"
	"errors"
	"kore-edge/internal/accounts"
	"kore-edge/internal/edgecatalog"
	"net/http"
	"strings"
)

func (a *API) partnerCatalog(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, ok := accounts.UserFromContext(r.Context())
	if !ok {
		w.WriteHeader(401)
		return
	}
	if a.PartnerCatalog == nil {
		w.WriteHeader(503)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/v1/kore-play/media/") {
		parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/api/v1/kore-play/media/"), "/", 3)
		if r.Method != "GET" || len(parts) != 3 {
			w.WriteHeader(400)
			return
		}
		if a.Live != nil && parts[1] == "http_ts" && parts[2] == "stream.ts" && a.Live.MediaIfActive(w, r, parts[0], parts[1]) {
			return
		}
		a.PartnerCatalog.Media(w, r, parts[0], parts[1], parts[2])
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var result any
	var e error
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/v1/kore-play/catalog":
		result = a.PartnerCatalog.Snapshot()
	case r.Method == "POST" && user.Role == "admin" && r.URL.Path == "/api/v1/kore-play/config":
		var input struct {
			CurrentPassword string             `json:"current_password"`
			Confirmed       bool               `json:"confirmed"`
			Generation      uint64             `json:"generation"`
			Config          edgecatalog.Config `json:"config"`
		}
		if !setupBody(r, &input) || !input.Confirmed {
			w.WriteHeader(400)
			return
		}
		input.Config.Origin = strings.TrimRight(strings.TrimSpace(input.Config.Origin), "/")
		e = a.Accounts.WireGuardChange(user, input.CurrentPassword, "partner_catalog_config", func() error { return a.PartnerCatalog.Configure(input.Config, input.Generation) })
		result = a.PartnerCatalog.Snapshot()
	case r.Method == "POST" && user.Role == "admin" && r.URL.Path == "/api/v1/kore-play/sync":
		var input struct {
			CurrentPassword string `json:"current_password"`
			Confirmed       bool   `json:"confirmed"`
		}
		if !setupBody(r, &input) || !input.Confirmed {
			w.WriteHeader(400)
			return
		}
		e = a.Accounts.WireGuardChange(user, input.CurrentPassword, "partner_catalog_sync", func() error { return a.PartnerCatalog.Sync(r.Context()) })
		result = a.PartnerCatalog.Snapshot()
	case r.Method == "POST" && user.Role == "admin" && r.URL.Path == "/api/v1/kore-play/probe":
		var input struct {
			Channel  string `json:"channel"`
			Protocol string `json:"protocol"`
		}
		if !setupBody(r, &input) {
			w.WriteHeader(400)
			return
		}
		result, e = a.PartnerCatalog.Probe(r.Context(), input.Channel, input.Protocol)
	default:
		w.WriteHeader(403)
		return
	}
	if e != nil {
		code := 503
		message := "Não foi possível consultar o Kore Play. Confira o endereço HTTPS, a rota do túnel e o certificado da central."
		switch {
		case errors.Is(e, accounts.ErrRejected):
			code = 403
			message = "Use a senha administrativa deste Edge."
		case errors.Is(e, edgecatalog.ErrInvalid):
			code = 400
			message = "Confirme o endereço HTTPS e um IP da central dentro das rotas WireGuard autorizadas."
		case errors.Is(e, edgecatalog.ErrConflict):
			code = 409
			message = "Há outra operação ou a configuração mudou. Atualize antes de continuar."
		case errors.Is(e, edgecatalog.ErrDenied):
			code = 403
			message = "Parceiro ou canal não autorizado. Confira as permissões na central."
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}
