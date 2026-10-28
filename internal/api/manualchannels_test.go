package api

import (
	"bytes"
	"context"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"io"
	"kore-edge/internal/accounts"
	"kore-edge/internal/live"
	"kore-edge/internal/manualchannels"
	"kore-edge/internal/playback"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestManualChannelsSessionCSRFRevisionAndSecretBoundary(t *testing.T) {
	directory := t.TempDir()
	users, e := accounts.Open(filepath.Join(directory, "accounts"))
	if e != nil {
		t.Fatal(e)
	}
	defer users.Close()
	password := "synthetic-admin-password"
	hash, _ := bcrypt.GenerateFromPassword([]byte(password), 10)
	if users.BootstrapHash("owner", string(hash)) != nil {
		t.Fatal("bootstrap")
	}
	sources, e := manualchannels.New(filepath.Join(directory, "manual"), nil)
	if e != nil {
		t.Fatal(e)
	}
	manager, e := live.New(context.Background(), filepath.Join(directory, "live"), apiManualCatalog{sources})
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	outputLinks, e := playback.Open(filepath.Join(directory, "playback"))
	if e != nil {
		t.Fatal(e)
	}
	h := (&API{Playback: outputLinks, ManualChannels: sources, Live: manager, Accounts: users, PublicOrigin: "https://edge.example"}).Handler()
	var cookie *http.Cookie
	csrf := ""
	call := func(method, path string, value any, token string) *httptest.ResponseRecorder {
		b, _ := json.Marshal(value)
		r := httptest.NewRequest(method, path, bytes.NewReader(b))
		r.Host = "127.0.0.1:18080"
		r.RemoteAddr = "127.0.0.1:12345"
		r.Header.Set("Origin", "https://edge.example")
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", token)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if w := call("GET", "/api/v1/manual-channels", nil, ""); w.Code != 401 {
		t.Fatal("anonymous manual access", w.Code)
	}
	login := func(name string) {
		w := call("POST", "/api/v1/auth/login", map[string]any{"username": name, "password": password}, "")
		if w.Code != 200 {
			t.Fatal("login", w.Code)
		}
		cookie = w.Result().Cookies()[0]
		var data struct {
			CSRF string `json:"csrf"`
		}
		json.Unmarshal(w.Body.Bytes(), &data)
		csrf = data.CSRF
	}
	login("owner")
	input := map[string]any{"action": "save", "revision": 0, "confirmed": true, "source": map[string]any{"name": "Synthetic", "url": "https://synthetic-user:synthetic-password@media.example/CHANNEL/mpegts?token=synthetic-token", "enabled": true}}
	if w := call("POST", "/api/v1/manual-channels", input, ""); w.Code != 403 {
		t.Fatal("missing CSRF", w.Code)
	}
	w := call("POST", "/api/v1/manual-channels", input, csrf)
	if w.Code != 200 {
		t.Fatal("save with session and no extra password", w.Code, w.Body.String())
	}
	var value struct {
		Revision  uint64 `json:"revision"`
		ChannelID string `json:"channel_id"`
		Prepared  bool   `json:"prepared"`
		Started   bool   `json:"started"`
	}
	if json.Unmarshal(w.Body.Bytes(), &value) != nil || value.ChannelID == "" || !value.Prepared || !value.Started || value.Revision != 1 {
		t.Fatal("default reception not prepared")
	}
	metadata := map[string]any{"action": "metadata", "revision": 1, "confirmed": true, "source": map[string]any{"id": value.ChannelID}, "metadata": map[string]any{"name": "Synthetic renamed", "title": "Synthetic title", "description": "Synthetic description"}}
	if w = call("POST", "/api/v1/manual-channels", metadata, ""); w.Code != 403 {
		t.Fatal("metadata bypassed CSRF", w.Code)
	}
	generation := manager.Snapshot().Configuration.Generation
	if w = call("POST", "/api/v1/manual-channels", metadata, csrf); w.Code != 200 {
		t.Fatal("active metadata failed", w.Code, w.Body.String())
	}
	if !manager.IsActive(value.ChannelID) || manager.Snapshot().Configuration.Generation != generation {
		t.Fatal("metadata restarted or stopped reception")
	}
	if w = call("POST", "/api/v1/manual-channels", metadata, csrf); w.Code != 409 {
		t.Fatal("stale metadata overwrite", w.Code)
	}
	profilePath := "/api/v1/manual-channels/processing?id=" + value.ChannelID
	if w = call("GET", profilePath, nil, ""); w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("synthetic-password")) {
		t.Fatal("profile read or secret boundary", w.Code)
	}
	inputs := map[string]any{"action": "inputs", "revision": 2, "confirmed": true, "source": map[string]any{"id": value.ChannelID}, "inputs": []map[string]any{{"key": "primary", "protocol": "http_ts"}, {"url": "https://backup.example/synthetic.ts", "protocol": "http_ts"}}}
	if w = call("POST", "/api/v1/manual-channels", inputs, ""); w.Code != 403 {
		t.Fatal("inputs bypassed CSRF", w.Code)
	}
	if w = call("POST", "/api/v1/manual-channels", inputs, csrf); w.Code != 200 {
		t.Fatal("active input configuration failed", w.Code, w.Body.String())
	}
	if !manager.IsActive(value.ChannelID) || manager.Snapshot().Configuration.Generation != generation+1 {
		t.Fatal("selected channel was not restarted")
	}
	if w = call("POST", "/api/v1/manual-channels", inputs, csrf); w.Code != 409 {
		t.Fatal("stale inputs accepted", w.Code)
	}
	for _, path := range []string{"/api/v1/manual-channels", "/api/v1/live"} {
		w = call("GET", path, nil, "")
		if w.Code != 200 || bytes.Contains(w.Body.Bytes(), []byte("synthetic-password")) || bytes.Contains(w.Body.Bytes(), []byte("synthetic-token")) {
			t.Fatal("secret leak or inventory failure", path)
		}
	}
	if w = call("POST", "/api/v1/manual-channels", input, csrf); w.Code != 409 {
		t.Fatal("stale overwrite", w.Code)
	}
	if w = call("POST", "/api/v1/live/action", map[string]any{"action": "start", "channel_id": value.ChannelID, "generation": 1, "manual_revision": 0, "confirmed": true}, csrf); w.Code != 409 {
		t.Fatal("stale source review", w.Code)
	}
	if w = call("POST", "/api/v1/live/action", map[string]any{"action": "stop", "channel_id": value.ChannelID, "confirmed": true}, csrf); w.Code != 200 {
		t.Fatal("stop without repeated password", w.Code)
	}
	paused := manager.Snapshot().Configuration.Channels[0]
	copyMode := map[string]any{"action": "transcode", "revision": 3, "confirmed": true, "source": map[string]any{"id": value.ChannelID}, "transcode": map[string]any{"mode": "copy"}}
	if w = call("POST", "/api/v1/manual-channels", copyMode, csrf); w.Code != 200 {
		t.Fatal("copy configuration failed", w.Code, w.Body.String())
	}
	after := manager.Snapshot().Configuration.Channels[0]
	if manager.IsActive(value.ChannelID) || after.Resume != paused.Resume || after.AutoPaused != paused.AutoPaused {
		t.Fatal("processing resumed a manually paused channel")
	}
	if w = call("POST", "/api/v1/users", map[string]any{"username": "reader", "password": password, "role": "viewer", "current_password": password}, csrf); w.Code != 204 {
		t.Fatal("viewer creation", w.Code)
	}
	login("reader")
	if w = call("GET", profilePath, nil, ""); w.Code != 403 {
		t.Fatal("viewer read protected profile", w.Code)
	}
	if w = call("POST", "/api/v1/manual-channels", copyMode, csrf); w.Code != 403 {
		t.Fatal("viewer changed processing", w.Code)
	}
	if w = call("POST", "/api/v1/manual-channels", metadata, csrf); w.Code != 403 {
		t.Fatal("viewer metadata accepted", w.Code)
	}
	if w = call("POST", "/api/v1/manual-channels", input, csrf); w.Code != 403 {
		t.Fatal("viewer write accepted", w.Code)
	}
	if w = call("POST", "/api/v1/live/action", map[string]any{"action": "stop", "channel_id": value.ChannelID, "confirmed": true}, csrf); w.Code != 403 {
		t.Fatal("viewer stream operation", w.Code)
	}
	login("owner")
	oldLink, e := outputLinks.Permanent(value.ChannelID)
	if e != nil {
		t.Fatal(e)
	}
	if w = call("POST", "/api/v1/manual-channels", map[string]any{"action": "delete", "revision": 3, "confirmed": true, "source": map[string]any{"id": value.ChannelID}}, csrf); w.Code != 200 {
		t.Fatal("delete", w.Code)
	}
	if _, e := outputLinks.Verify(oldLink, value.ChannelID, false); e == nil {
		t.Fatal("deleted channel kept its permanent link")
	}
	if sources.Authorized(value.ChannelID, "http_ts") || len(manager.Snapshot().Configuration.Channels) != 0 {
		t.Fatal("deleted channel retained grant/config")
	}
	if w = call("POST", "/api/v1/auth/logout", nil, csrf); w.Code != 204 {
		t.Fatal("logout")
	}
	if w = call("POST", "/api/v1/manual-channels", input, csrf); w.Code != 401 {
		t.Fatal("revoked session accepted", w.Code)
	}
}

type apiManualCatalog struct{ *manualchannels.Service }

func (c apiManualCatalog) OpenHTTP(ctx context.Context, _ string) (io.ReadCloser, error) {
	r, w := io.Pipe()
	go func() { <-ctx.Done(); w.Close() }()
	return r, nil
}
