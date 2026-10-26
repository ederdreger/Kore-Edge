package hls

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestCodecPolicy(t *testing.T) {
	for _, tc := range []struct {
		input string
		ok    bool
	}{
		{`{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"aac"}]}`, true},
		{`{"streams":[{"codec_type":"video","codec_name":"hevc"},{"codec_type":"audio","codec_name":"aac"}]}`, false},
		{`{"streams":[{"codec_type":"video","codec_name":"h264"},{"codec_type":"audio","codec_name":"mp2"}]}`, false},
		{`{"streams":[]}`, false}, {`not-json`, false},
	} {
		if (ValidateCodecs([]byte(tc.input)) == nil) != tc.ok {
			t.Fatal("codec policy")
		}
	}
	args := strings.Join(segmentArgs(), " ")
	if !strings.Contains(args, "-c copy") || strings.Contains(args, "libx264") || !strings.Contains(args, "delete_segments+temp_file") {
		t.Fatal("HLS must remux")
	}
}

func TestHTTPAuthorizationAndPaths(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)
	m, e := newStorage(Config{Root: t.TempDir()}, func(string, time.Time) bool { return allowed.Load() })
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	dir, e := os.MkdirTemp(m.root, "test-")
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	close(done)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &Session{owner: m, id: "test", dir: dir, done: done, ctx: ctx, cancel: cancel}
	m.sessions["test"] = s
	if os.WriteFile(filepath.Join(dir, "index.m3u8"), []byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXTINF:2,\nseg000000001.ts\n"), 0600) != nil {
		t.Fatal("playlist")
	}
	if os.WriteFile(filepath.Join(dir, "seg000000001.ts"), make([]byte, 188), 0600) != nil {
		t.Fatal("segment")
	}
	for _, tc := range []struct {
		method, path string
		code         int
	}{
		{"GET", "/hls/test/index.m3u8", 200}, {"GET", "/hls/test/seg000000001.ts", 200},
		{"HEAD", "/hls/test/index.m3u8", 200}, {"POST", "/hls/test/index.m3u8", 405},
		{"GET", "/hls/test/../secret", 404}, {"GET", "/hls/test/", 404}, {"GET", "/hls/test/index.m3u8.tmp", 404},
		{"GET", "/hls/test/seg000000002.ts", 404},
	} {
		w := httptest.NewRecorder()
		m.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.code {
			t.Fatalf("%s: %d", tc.path, w.Code)
		}
		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("cache")
		}
		if tc.method == "HEAD" && w.Body.Len() != 0 {
			t.Fatal("HEAD")
		}
	}
	allowed.Store(false)
	w := httptest.NewRecorder()
	m.ServeHTTP(w, httptest.NewRequest("GET", "/hls/test/seg000000001.ts", nil))
	if w.Code != 403 {
		t.Fatal("revocation")
	}
	allowed.Store(true)
	if s.Close() != nil {
		t.Fatal("close")
	}
	if _, e := os.Stat(dir); !os.IsNotExist(e) {
		t.Fatal("segments retained after stop")
	}
	w = httptest.NewRecorder()
	m.ServeHTTP(w, httptest.NewRequest("GET", "/hls/test/seg000000001.ts", nil))
	if w.Code != 404 {
		t.Fatal("stale segment served")
	}
}
func TestCleanupScopeAndConfig(t *testing.T) {
	root := t.TempDir()
	other := t.TempDir()
	if removeOwned(root, root) == nil || removeOwned(root, other) == nil {
		t.Fatal("unsafe cleanup")
	}
	if _, e := os.Stat(other); e != nil {
		t.Fatal("removed unrelated folder")
	}
	if (Config{Enabled: true, FFmpeg: "ffmpeg", FFprobe: "ffprobe", Root: root}).Validate() == nil {
		t.Fatal("relative tools")
	}
	if (Config{}).Validate() != nil {
		t.Fatal("disabled")
	}
}
func TestPlayerLocalAssets(t *testing.T) {
	m := &Manager{allow: func(string, time.Time) bool { return true }}
	for _, path := range []string{"/player/test", "/player-assets/hls.min.js", "/player-assets/player.js", "/player-assets/player.css"} {
		w := httptest.NewRecorder()
		m.Player(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Body.Len() == 0 || w.Header().Get("Content-Security-Policy") == "" {
			t.Fatal("player asset")
		}
	}
	w := httptest.NewRecorder()
	m.Player(w, httptest.NewRequest("GET", "/player/../test", nil))
	if w.Code != 404 {
		t.Fatal("player path traversal")
	}
}

func TestStorageWatchdogAndQueueBounds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	m, e := newStorage(Config{Root: t.TempDir()}, func(string, time.Time) bool { return true })
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	dir, e := os.MkdirTemp(m.root, "watchdog-")
	if e != nil {
		t.Fatal(e)
	}
	done := make(chan struct{})
	close(done)
	s := &Session{owner: m, id: "test", dir: dir, ctx: ctx, cancel: cancel, queue: make(chan [188]byte, 128), done: done}
	m.sessions["test"] = s
	f, e := os.Create(filepath.Join(dir, "oversize.ts"))
	if e != nil {
		t.Fatal(e)
	}
	if f.Truncate(MaxSegmentBytes+1) != nil {
		t.Fatal("fixture")
	}
	f.Close()
	go s.monitor()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("quota watchdog did not cancel")
	}
	if s.WritePacket(make([]byte, 188)) == nil {
		t.Fatal("invalid packet accepted")
	}
}
