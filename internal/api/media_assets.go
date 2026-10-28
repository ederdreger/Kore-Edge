package api

import (
	"encoding/json"
	"io"
	"kore-edge/internal/accounts"
	"kore-edge/internal/catalog"
	"net/http"
	"time"
)

func (a *API) mediaAssets(w http.ResponseWriter, r *http.Request, actor accounts.User) {
	if r.Method == "GET" {
		assets, e := a.ManualChannels.Assets()
		if e != nil {
			w.WriteHeader(503)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"assets": assets})
		return
	}
	if r.Method != "POST" {
		w.WriteHeader(405)
		return
	}
	var input struct {
		Action string `json:"action"`
		Kind   string `json:"kind"`
		URL    string `json:"url"`
		ID     string `json:"id"`
	}
	body, e := io.ReadAll(io.LimitReader(r.Body, 8193))
	if e != nil || len(body) > 8192 || catalog.DecodeStrict(body, &input) != nil {
		w.WriteHeader(400)
		return
	}
	if input.Action != "import" && input.Action != "delete" {
		w.WriteHeader(400)
		return
	}
	http.NewResponseController(w).SetWriteDeadline(time.Now().Add(60 * time.Second))
	var result any
	e = a.Accounts.ChannelChange(actor, "media_asset_"+input.Action, func() error {
		switch input.Action {
		case "import":
			cap, e := a.Live.Capabilities()
			if e != nil {
				return e
			}
			asset, e := a.ManualChannels.ImportAsset(r.Context(), input.Kind, input.URL, cap.Tools.FFprobe)
			result = asset
			return e
		case "delete":
			return a.ManualChannels.DeleteAsset(input.ID)
		default:
			return io.ErrUnexpectedEOF
		}
	})
	if e != nil {
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(map[string]string{"error": "Arquivo recusado. Confira URL, formato, tamanho, recursos disponíveis e se está em uso."})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"asset": result})
}
