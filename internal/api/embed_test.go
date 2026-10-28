package api

import (
	"context"
	"kore-edge/internal/accounts"
	"kore-edge/internal/live"
	"kore-edge/internal/playback"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapabilityEmbedAndReadOnlyInfoNeverExposeAdministration(t *testing.T) {
	root := t.TempDir()
	users, e := accounts.Open(filepath.Join(root, "users"))
	if e != nil {
		t.Fatal(e)
	}
	defer users.Close()
	keys, e := playback.Open(filepath.Join(root, "playback"))
	if e != nil {
		t.Fatal(e)
	}
	source := &vlcCatalog{}
	source.allowed.Store(true)
	manager, e := live.New(context.Background(), filepath.Join(root, "live"), source)
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	if manager.Save(live.ChannelConfig{ChannelID: "synthetic", Protocol: "http_ts", HLS: true}, live.Tools{}, 0) != nil || manager.Start("synthetic") != nil {
		t.Fatal("prepare")
	}
	key, _ := keys.Permanent("synthetic")
	base := "/api/v1/playback/" + key + "/synthetic/"
	h := (&API{Accounts: users, Playback: keys, Live: manager, PublicOrigin: "https://edge.example"}).Handler()
	get := func(path, site string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", path, nil)
		r.RemoteAddr = "127.0.0.1:12345"
		r.Header.Set("Sec-Fetch-Site", site)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	w := get(base+"embed.html", "cross-site")
	if w.Code != 200 || !strings.Contains(w.Header().Get("Content-Security-Policy"), "frame-ancestors *") || !strings.Contains(w.Body.String(), "<video") {
		t.Fatal("capability embed denied", w.Code)
	}
	if w = get(base+"embed.js", "same-origin"); w.Code != 200 || !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
		t.Fatal("embed script")
	}
	if w = get(base+"hls.js", "same-origin"); w.Code != 200 || w.Body.Len() < 10000 {
		t.Fatal("missing bundled library")
	}
	if get(strings.Replace(base, key, "invalid", 1)+"embed.html", "cross-site").Code != 401 {
		t.Fatal("embed without capability")
	}
	if get(base+"stream.ts", "cross-site").Code != 400 {
		t.Fatal("embed exemption widened to media")
	}
	if get(base+"cmaf/hls.js", "cross-site").Code != 404 {
		t.Fatal("embed exemption widened to nested paths")
	}
	if w = get(base+"media_info.json", "same-origin"); w.Code != 200 || !strings.Contains(w.Body.String(), "bits_per_second") || strings.Contains(w.Body.String(), "url") {
		t.Fatal("read-only info failed", w.Code)
	}
	source.allowed.Store(false)
	if get(base+"embed.html", "cross-site").Code != 401 {
		t.Fatal("revoked channel embed")
	}
}
