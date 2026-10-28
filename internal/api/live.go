package api

import (
	"encoding/json"
	"errors"
	"kore-edge/internal/accounts"
	"kore-edge/internal/live"
	"kore-edge/internal/multicast"
	"net/http"
	"strings"
)

func (a *API) liveHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	user, ok := accounts.UserFromContext(r.Context())
	if !ok {
		w.WriteHeader(401)
		return
	}
	if a.Live == nil {
		w.WriteHeader(503)
		return
	}
	for _, kind := range []string{"hls", "media"} {
		prefix := "/api/v1/live/" + kind + "/"
		if strings.HasPrefix(r.URL.Path, prefix) {
			parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, prefix), "/", 2)
			if r.Method != "GET" || len(parts) != 2 || r.URL.RawQuery != "" {
				w.WriteHeader(400)
				return
			}
			if kind == "hls" {
				a.Live.ServeHLS(w, r, parts[0], parts[1])
			} else if parts[1] == "preview.jpg" {
				a.Live.ServeThumbnail(w, r, parts[0])
			} else if parts[1] == "stream.ts" {
				a.Live.ServeTS(w, r, parts[0])
			} else {
				w.WriteHeader(404)
			}
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	var result any
	var e error
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/v1/live":
		result = a.Live.Snapshot()
	case r.Method == "GET" && r.URL.Path == "/api/v1/live/capabilities":
		result, e = a.Live.Capabilities()
	case r.Method == "POST" && r.URL.Path == "/api/v1/live/udp-plan" && user.Role == "admin":
		var target multicast.Target
		if !setupBody(r, &target) {
			w.WriteHeader(400)
			return
		}
		result, e = a.Live.PlanUDP(target)
	case r.Method == "POST" && r.URL.Path == "/api/v1/live/action" && user.Role == "admin":
		var input struct {
			Action          string             `json:"action"`
			CurrentPassword string             `json:"current_password"`
			Confirmed       bool               `json:"confirmed"`
			Generation      uint64             `json:"generation"`
			ManualRevision  uint64             `json:"manual_revision"`
			Channel         live.ChannelConfig `json:"channel"`
			ChannelID       string             `json:"channel_id"`
			Tools           live.Tools         `json:"tools"`
		}
		if !setupBody(r, &input) || !input.Confirmed {
			w.WriteHeader(400)
			return
		}
		if input.Action != "stop" && a.ManualChannels != nil && input.ManualRevision != a.ManualChannels.Revision() {
			w.WriteHeader(409)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "O cadastro manual mudou. Atualize e revise o canal novamente."})
			return
		}
		reviewed := func(change func() error) error {
			if a.ManualChannels != nil && input.ManualRevision != a.ManualChannels.Revision() {
				return live.ErrConflict
			}
			return change()
		}
		switch input.Action {
		case "save":
			e = a.Accounts.ChannelChange(user, "live_config", func() error {
				return reviewed(func() error { return a.Live.Save(input.Channel, input.Tools, input.Generation) })
			})
		case "start":
			e = a.Accounts.ChannelChange(user, "live_start", func() error {
				return reviewed(func() error { return a.Live.StartReviewed(input.ChannelID, input.Generation) })
			})
		case "stop":
			e = a.Accounts.ChannelChange(user, "live_stop", func() error { return a.Live.Stop(input.ChannelID) })
		default:
			w.WriteHeader(400)
			return
		}
		result = a.Live.Snapshot()
	default:
		w.WriteHeader(403)
		return
	}
	if e != nil {
		code := 503
		message := "Operação indisponível. Confira a origem, as ferramentas e a configuração do canal."
		switch {
		case errors.Is(e, accounts.ErrBusy):
			code = 429
			message = "Há outra operação de canais em andamento. Aguarde antes de tentar novamente."
		case errors.Is(e, accounts.ErrRejected):
			code = 403
			message = "Esta operação exige uma sessão administrativa válida."
		case errors.Is(e, live.ErrDenied):
			code = 403
			message = "Canal, protocolo ou saída não autorizado. Revise a liberação do parceiro no Kore Play."
		case errors.Is(e, live.ErrConflict):
			code = 409
			message = "A configuração mudou, as ferramentas estão em uso ou o limite foi atingido. Atualize a tela e confira o limite de canais ativos."
		case errors.Is(e, live.ErrInvalid):
			code = 400
			message = "Confira o canal, o protocolo, os caminhos detectados e o destino/interface da saída."
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}
