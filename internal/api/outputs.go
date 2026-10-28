package api

import (
	"encoding/json"
	"errors"
	"kore-edge/internal/accounts"
	"kore-edge/internal/output"
	"net/http"
)

func (a *API) outputsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Referrer-Policy", "no-referrer")
	actor, ok := accounts.UserFromContext(r.Context())
	if !ok {
		w.WriteHeader(401)
		return
	}
	if actor.Role != "admin" {
		w.WriteHeader(403)
		return
	}
	if a.Outputs == nil || a.Live == nil {
		w.WriteHeader(503)
		return
	}
	id := r.URL.Query().Get("id")
	known := false
	view := a.Live.Snapshot()
	if view.Catalog.Catalog != nil {
		for _, c := range view.Catalog.Catalog.Channels {
			if c.ID == id {
				known = true
			}
		}
	}
	if !known {
		w.WriteHeader(404)
		return
	}
	if r.Method == "GET" {
		json.NewEncoder(w).Encode(map[string]any{"configuration": a.Outputs.Snapshot(id), "links": a.Outputs.Links(id)})
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var input struct {
		Revision  uint64          `json:"revision"`
		Channel   output.Channel  `json:"channel"`
		Network   *output.Network `json:"network"`
		Confirmed bool            `json:"confirmed"`
	}
	if !setupBody(r, &input) || !input.Confirmed {
		w.WriteHeader(400)
		return
	}
	err := a.Accounts.ChannelChange(actor, "live_config", func() error { return a.Outputs.Save(id, input.Channel, input.Network, input.Revision) })
	if err != nil {
		code := 503
		message := "Saída indisponível. Confira as ferramentas de mídia e o estado da recepção."
		if errors.Is(err, output.ErrConflict) {
			code = 409
			message = "A configuração de saídas mudou. Recarregue antes de salvar."
		}
		if errors.Is(err, output.ErrInvalid) {
			code = 400
			message = "Revise o IP local, domínio/IP anunciado, portas, protocolos e limites. Confirme a origem do canal antes de configurar as saídas."
		}
		if errors.Is(err, accounts.ErrBusy) {
			code = 429
			message = "Aguarde a operação de canais em andamento."
		}
		w.WriteHeader(code)
		json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"configuration": a.Outputs.Snapshot(id), "links": a.Outputs.Links(id)})
}
