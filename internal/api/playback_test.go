package api

import (
	"bytes"
	"context"
	"encoding/json"
	"golang.org/x/crypto/bcrypt"
	"io"
	"kore-edge/internal/accounts"
	"kore-edge/internal/edgecatalog"
	"kore-edge/internal/live"
	"kore-edge/internal/playback"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type vlcCatalog struct {
	allowed atomic.Bool
	opens   atomic.Int32
	data    []byte
}

func (c *vlcCatalog) Snapshot() edgecatalog.Snapshot {
	return edgecatalog.Snapshot{Available: c.allowed.Load()}
}
func (c *vlcCatalog) Authorized(id, p string) bool {
	return c.allowed.Load() && id == "synthetic" && p == "http_ts"
}
func (c *vlcCatalog) OutputAuthorized(string, string) bool { return c.allowed.Load() }
func (c *vlcCatalog) OpenSRT(context.Context, string) (io.ReadCloser, error) {
	return nil, live.ErrDenied
}
func (c *vlcCatalog) OpenHLS(context.Context, string, string) (io.ReadCloser, error) {
	return nil, live.ErrDenied
}
func (c *vlcCatalog) OpenHTTP(ctx context.Context, _ string) (io.ReadCloser, error) {
	c.opens.Add(1)
	r, w := io.Pipe()
	data := c.data
	if len(data) == 0 {
		packet := make([]byte, 188)
		packet[0] = 0x47
		packet[1] = 0x1f
		packet[2] = 0xff
		packet[3] = 0x10
		data = bytes.Repeat(packet, 7)
	}
	go func() {
		defer w.Close()
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(time.Millisecond):
				if _, e := w.Write(data); e != nil {
					return
				}
			}
		}
	}()
	return r, nil
}

func TestVLCTokenStreamsWithoutCookieAndRevokesOpenConnection(t *testing.T) {
	dir := t.TempDir()
	users, e := accounts.Open(filepath.Join(dir, "users"))
	if e != nil {
		t.Fatal(e)
	}
	defer users.Close()
	hash, _ := bcrypt.GenerateFromPassword([]byte("synthetic-vlc-password"), 10)
	if users.BootstrapHash("owner", string(hash)) != nil {
		t.Fatal("bootstrap")
	}
	store, e := playback.Open(filepath.Join(dir, "playback"))
	if e != nil {
		t.Fatal(e)
	}
	c := &vlcCatalog{}
	c.allowed.Store(true)
	m, e := live.New(context.Background(), filepath.Join(dir, "live"), c)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if m.Save(live.ChannelConfig{ChannelID: "synthetic", Protocol: "http_ts"}, live.Tools{}, 0) != nil || m.Start("synthetic") != nil {
		t.Fatal("prepare synthetic source")
	}
	api := &API{Accounts: users, Playback: store, Live: m, PublicOrigin: "https://edge.example"}
	h := api.Handler()
	request := func(method, path, body, csrf string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:12345"
		r.Host = "127.0.0.1:18080"
		r.Header.Set("Origin", api.PublicOrigin)
		r.Header.Set("Content-Type", "application/json")
		r.Header.Set("X-CSRF-Token", csrf)
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	login := request("POST", "/api/v1/auth/login", `{"username":"owner","password":"synthetic-vlc-password"}`, "", nil)
	if login.Code != 200 {
		t.Fatal("login")
	}
	cookie := login.Result().Cookies()[0]
	var session struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(login.Body.Bytes(), &session)
	body := `{"action":"issue","channel_ids":["synthetic"]}`
	if request("POST", "/api/v1/live/playback-links", body, "", nil).Code != 401 || request("POST", "/api/v1/live/playback-links", body, "", cookie).Code != 403 {
		t.Fatal("issue without session/CSRF")
	}
	issued := request("POST", "/api/v1/live/playback-links", body, session.CSRF, cookie)
	if issued.Code != 200 {
		t.Fatal("issue", issued.Code)
	}
	var value struct {
		Links []struct {
			TS  string `json:"ts_path"`
			HLS string `json:"hls_path"`
		} `json:"links"`
	}
	_ = json.Unmarshal(issued.Body.Bytes(), &value)
	if len(value.Links) != 1 || value.Links[0].HLS != "" {
		t.Fatal("unconfigured HLS advertised")
	}
	path := value.Links[0].TS
	for _, bad := range []string{strings.Replace(path, "/synthetic/", "/other/", 1), path + "?token=other", strings.Replace(path, "/stream.ts", "/%2e%2e/stream.ts", 1)} {
		if request("GET", bad, "", "", nil).Code == 200 {
			t.Fatal("invalid scope accepted")
		}
	}
	for _, tail := range []string{"index.m3u8", "seg000000001.ts"} {
		if request("GET", strings.TrimSuffix(path, "stream.ts")+tail, "", "", nil).Code != 401 {
			t.Fatal("TS token expanded to HLS")
		}
	}
	deadline := time.Now().Add(4 * time.Second)
	for !m.PlaybackReady("synthetic") && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	server := httptest.NewTLSServer(h)
	defer server.Close()
	client := server.Client()
	client.Timeout = 5 * time.Second
	resp, e := client.Get(server.URL + path)
	if e != nil {
		t.Fatal("VLC-style cookie-free request failed")
	}
	defer resp.Body.Close()
	data := make([]byte, 188*14)
	_, e = io.ReadFull(resp.Body, data)
	if e != nil || resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "video/mp2t" {
		t.Fatal("not a TS response", resp.StatusCode, e)
	}
	for i := 0; i < len(data); i += 188 {
		if data[i] != 0x47 {
			t.Fatal("broken MPEG-TS framing")
		}
	}
	if c.opens.Load() != 1 {
		t.Fatal("external player opened another origin")
	}
	head, e := client.Head(server.URL + path)
	if e != nil {
		t.Fatal("HEAD failed")
	}
	head.Body.Close()
	if head.StatusCode != 200 || head.Header.Get("Content-Type") != "video/mp2t" {
		t.Fatal("HEAD not compatible", head.StatusCode)
	}
	owner := users.Users()[0]
	wrong, _, e := store.Issue("synthetic", owner.ID, owner.Revision+1, false)
	if e != nil {
		t.Fatal(e)
	}
	if request("GET", "/api/v1/playback/"+wrong+"/synthetic/stream.ts", "", "", nil).Code != 401 {
		t.Fatal("obsolete owner revision accepted")
	}

	if request("GET", "/api/v1/live", "", "", nil).Code != 401 {
		t.Fatal("capability became panel session")
	}
	revoked := request("POST", "/api/v1/live/playback-links", `{"action":"revoke","channel_ids":["synthetic"]}`, session.CSRF, cookie)
	if revoked.Code != 200 {
		t.Fatal("revoke failed")
	}
	stoppedAt := time.Now()
	_, _ = io.Copy(io.Discard, resp.Body)
	if time.Since(stoppedAt) > 3*time.Second {
		t.Fatal("revocation did not close the running player")
	}
	if request("GET", path, "", "", nil).Code != 401 {
		t.Fatal("revoked link accepted")
	}
	if c.opens.Load() != 1 || !m.IsActive("synthetic") {
		t.Fatal("revoking player stopped source")
	}
	fresh, _, e := store.Issue("synthetic", owner.ID, owner.Revision, false)
	if e != nil {
		t.Fatal(e)
	}
	c.allowed.Store(false)
	if request("GET", "/api/v1/playback/"+fresh+"/synthetic/stream.ts", "", "", nil).Code != 401 {
		t.Fatal("revoked source grant accepted")
	}
	if request("POST", "/api/v1/live/playback-links", `{"action":"revoke","channel_ids":["synthetic"]}`, session.CSRF, cookie).Code != 200 {
		t.Fatal("cannot revoke after source grant expired")
	}
	create := request("POST", "/api/v1/users", `{"username":"viewer","password":"synthetic-vlc-password","role":"viewer","current_password":"synthetic-vlc-password"}`, session.CSRF, cookie)
	if create.Code != 204 {
		t.Fatal("viewer setup", create.Code)
	}
	viewerLogin := request("POST", "/api/v1/auth/login", `{"username":"viewer","password":"synthetic-vlc-password"}`, "", nil)
	if viewerLogin.Code != 200 {
		t.Fatal("viewer login")
	}
	var viewerSession struct {
		CSRF string `json:"csrf"`
	}
	_ = json.Unmarshal(viewerLogin.Body.Bytes(), &viewerSession)
	if request("POST", "/api/v1/live/playback-links", body, viewerSession.CSRF, viewerLogin.Result().Cookies()[0]).Code != 403 {
		t.Fatal("spectator issued external credentials")
	}

}
