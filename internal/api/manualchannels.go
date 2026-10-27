package api

import (
	"encoding/json"
	"errors"
	"kore-edge/internal/accounts"
	"kore-edge/internal/live"
	"kore-edge/internal/manualchannels"
	"net/http"
	"time"
)

func (a *API) manualChannels(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	actor, ok := accounts.UserFromContext(r.Context())
	if !ok {
		w.WriteHeader(401)
		return
	}
	if actor.Role != "admin" {
		w.WriteHeader(403)
		return
	}
	if a.ManualChannels == nil || a.Live == nil {
		w.WriteHeader(503)
		return
	}
	if r.Method == "GET" && r.URL.Path == "/api/v1/manual-channels" {
		_ = json.NewEncoder(w).Encode(a.ManualChannels.Inventory())
		return
	}
	if r.Method != "POST" || r.URL.Path != "/api/v1/manual-channels" || r.URL.RawQuery != "" {
		w.WriteHeader(405)
		return
	}
	var input struct {
		Action    string                `json:"action"`
		Revision  uint64                `json:"revision"`
		Source    manualchannels.Source `json:"source"`
		Confirmed bool                  `json:"confirmed"`
	}
	if !setupBody(r, &input) {
		w.WriteHeader(400)
		return
	}
	var result any
	var e error
	switch input.Action {
	case "detect":
		result, e = manualchannels.Detect(input.Source.URL, input.Source.Protocol)
	case "probe":
		if !input.Confirmed {
			w.WriteHeader(400)
			return
		}
		// Only this bounded media test outlives the control API's normal 5 s
		// write timeout. Do not relax deadlines for unrelated endpoints.
		_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(12 * time.Second))
		e = a.Accounts.ChannelChange(actor, "manual_channel_probe", func() error {
			cap, err := a.Live.Capabilities()
			if err != nil {
				return err
			}
			value, err := a.ManualChannels.Probe(r.Context(), input.Source, cap.Tools.FFmpeg)
			result = value
			return err
		})
	case "save", "delete":
		if !input.Confirmed {
			w.WriteHeader(400)
			return
		}
		e = a.Accounts.ChannelChange(actor, "manual_channel_"+input.Action, func() error {
			if a.Live.IsActive(input.Source.ID) {
				return live.ErrConflict
			}
			if input.Revision != a.ManualChannels.Revision() {
				return manualchannels.ErrConflict
			}
			if input.Action == "delete" {
				if !a.ManualChannels.IsManual(input.Source.ID) {
					return manualchannels.ErrInvalid
				}
				if err := a.Live.Forget(input.Source.ID); err != nil {
					return err
				}
				if err := a.ManualChannels.Delete(input.Source.ID, input.Revision); err != nil {
					return err
				}
				if a.Playback != nil {
					if err := a.Playback.Revoke([]string{input.Source.ID}); err != nil {
						return err
					}
				}
			} else {
				id, err := a.ManualChannels.Save(input.Source, input.Revision)
				if err != nil {
					return err
				}
				inventory := a.ManualChannels.Inventory()
				var protocol string
				for _, c := range inventory.Channels {
					if c.ID == id {
						protocol = c.Protocol
					}
				}
				current := a.Live.Snapshot().Configuration
				config := live.ChannelConfig{ChannelID: id, Protocol: protocol, HLS: a.ManualChannels.OutputAuthorized(id, "hls"), Resume: input.Source.Enabled}
				for _, saved := range current.Channels {
					if saved.ChannelID == id {
						config = saved
						config.Protocol = protocol
					}
				}
				if input.Source.URL != "" {
					config.Resume = input.Source.Enabled
				}
				tools := current.Tools
				if tools.FFmpeg == "" {
					if cap, err := a.Live.Capabilities(); err == nil {
						tools = cap.Tools
					}
				}
				prepared := a.Live.Save(config, tools, current.Generation) == nil
				started := false
				if prepared && input.Source.Enabled {
					started = a.Live.StartReviewed(id, a.Live.Snapshot().Configuration.Generation) == nil
				}
				result = map[string]any{"revision": inventory.Revision, "channels": inventory.Channels, "channel_id": id, "prepared": prepared, "started": started}
				return nil
			}
			result = a.ManualChannels.Inventory()
			return nil
		})
	default:
		w.WriteHeader(400)
		return
	}
	if e != nil {
		code := 503
		message := "Não foi possível receber o sinal. Confira o link, a disponibilidade da origem e as ferramentas do Edge."
		switch {
		case errors.Is(e, accounts.ErrBusy):
			code = 429
			message = "Há outra operação de canais em andamento. Aguarde o teste ou a alteração anterior."
		case errors.Is(e, manualchannels.ErrInvalid):
			code = 400
			message = "Use um link HTTP MPEG-TS, HLS, UDP MPEG-TS ou SRT caller válido. Em UDP, confirme interface, IP local e porta; em SRT, a latência do link é em microssegundos."
		case errors.Is(e, manualchannels.ErrConflict) || errors.Is(e, live.ErrConflict):
			code = 409
			message = "O cadastro mudou ou o canal está ativo. Atualize e pare a recepção antes de editar ou excluir."
		case errors.Is(e, accounts.ErrRejected) || errors.Is(e, manualchannels.ErrDenied):
			code = 403
			message = "Sessão ou destino não autorizado. Não são aceitos loopback, metadados, redirecionamentos ou referências HLS externas."
		}
		w.WriteHeader(code)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
		return
	}
	_ = json.NewEncoder(w).Encode(result)
}
