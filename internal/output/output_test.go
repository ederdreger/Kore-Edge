package output

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"

	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

type testSource struct {
	active, allowed atomic.Bool
	opens           atomic.Int32
}

func (s *testSource) SharedStream(ctx context.Context, id string) (io.ReadCloser, error) {
	s.opens.Add(1)
	r, w := io.Pipe()
	go func() { <-ctx.Done(); w.Close() }()
	return r, nil
}
func (s *testSource) PlaybackAllowed(string, bool) bool { return s.allowed.Load() }
func (s *testSource) IsActive(string) bool              { return s.active.Load() }

type testKeys struct{}

func (testKeys) Permanent(id string) (string, error) {
	return "p1.synthetic-secret-for-tests-" + id, nil
}
func fixture(t *testing.T) (*Service, *testSource) {
	t.Helper()
	source := &testSource{}
	source.allowed.Store(true)
	s, e := New(context.Background(), filepath.Join(t.TempDir(), "outputs"), source, testKeys{})
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return s, source
}

func TestOutputSettingsDefaultDisabledRevisionAndPrivatePersistence(t *testing.T) {
	s, source := fixture(t)
	v := s.Snapshot("synthetic")
	if v.Network.Enabled || v.Channel.DASH || v.Channel.SRT || v.Status.Bridge != "disabled" {
		t.Fatal("output started implicitly")
	}
	c := DefaultChannel()
	c.Segments = 5
	if e := s.Save("synthetic", c, nil, 0); e != nil {
		t.Fatal(e)
	}
	if source.opens.Load() != 0 {
		t.Fatal("saving a paused channel pulled media")
	}
	if e := s.Save("synthetic", c, nil, 0); !errors.Is(e, ErrConflict) {
		t.Fatal("stale update accepted")
	}
	if e := s.Save("synthetic", c, nil, 1); e != nil || s.Snapshot("synthetic").Revision != 1 {
		t.Fatal("same configuration not idempotent")
	}
	copy, e := New(context.Background(), s.directory, source, testKeys{})
	if e != nil {
		t.Fatal(e)
	}
	defer copy.Close()
	if copy.Snapshot("synthetic").Channel.Segments != 5 {
		t.Fatal("settings lost")
	}
	if runtime.GOOS != "windows" {
		file := filepath.Join(s.directory, "outputs.json")
		info, _ := os.Stat(file)
		if info.Mode().Perm() != 0600 {
			t.Fatal("not private")
		}
		os.Chmod(file, 0644)
		if _, e = New(context.Background(), s.directory, source, testKeys{}); e == nil {
			t.Fatal("public configuration accepted")
		}
	}
}
func TestNetworkAndChannelValidation(t *testing.T) {
	for _, n := range []Network{{Enabled: true}, {Enabled: true, BindIP: "0.0.0.0", PublicHost: "edge.example", SRT: 20000}, {Enabled: true, BindIP: "192.0.2.10", PublicHost: "https://edge.example", SRT: 20000}, {Enabled: true, BindIP: "192.0.2.10", PublicHost: "edge.example", RTMP: 443}, {Enabled: true, BindIP: "192.0.2.10", PublicHost: "edge.example", RTMP: 20000, RTSP: 20000}} {
		if validateNetwork(n) == nil {
			t.Fatal("invalid listeners accepted")
		}
	}
	if validateNetwork(Network{Enabled: true, BindIP: "192.0.2.10", PublicHost: "edge.example", RTMP: 1935, RTSP: 8554, SRT: 20000}) != nil {
		t.Fatal("explicit listener configuration rejected")
	}
	s, _ := fixture(t)
	c := DefaultChannel()
	c.Segments = 999
	if s.Save("synthetic", c, nil, 0) == nil {
		t.Fatal("unbounded segmentation")
	}
	c = DefaultChannel()
	if s.Save("../../escape", c, nil, 0) == nil {
		t.Fatal("path injection")
	}
}
func TestBridgeAuthenticationIsReadOnlyChannelScopedAndProtocolScoped(t *testing.T) {
	s, source := fixture(t)
	source.active.Store(true)
	s.mu.Lock()
	s.data.Network.Enabled = true
	s.data.Channels["synthetic"] = Channel{RTSP: true, MaxReaders: 4, Duration: 2, Segments: 4}
	s.mu.Unlock()
	call := func(protocol, action, key string) int {
		b, _ := json.Marshal(map[string]string{"user": "edge", "password": key, "action": action, "path": "synthetic", "protocol": protocol})
		w := httptest.NewRecorder()
		s.authenticate(w, httptest.NewRequest("POST", "/auth", bytes.NewReader(b)))
		return w.Code
	}
	key, _ := s.keys.Permanent("synthetic")
	if call("rtsp", "read", key) != 204 {
		t.Fatal("authorized read denied")
	}
	if call("rtmp", "read", key) == 204 || call("rtsp", "publish", key) == 204 || call("rtsp", "read", "wrong") == 204 {
		t.Fatal("scope bypass")
	}
	source.allowed.Store(false)
	if call("rtsp", "read", key) == 204 {
		t.Fatal("revoked grant accepted")
	}
}
func TestGeneratedConfigHasNoAnonymousAccessHooksOrUnreviewedListeners(t *testing.T) {
	s, _ := fixture(t)
	s.mu.Lock()
	s.data.Network = Network{Enabled: true, BindIP: "192.0.2.10", PublicHost: "edge.example", RTSP: 18554, SRT: 20000}
	s.auth = &authServer{address: "http://127.0.0.1:12345/auth"}
	b, e := s.bridgeDocument(map[string]Channel{"synthetic": DefaultChannel()})
	s.auth = nil
	s.mu.Unlock()
	if e != nil {
		t.Fatal(e)
	}
	var conf map[string]any
	if json.Unmarshal(b, &conf) != nil {
		t.Fatal("config")
	}
	if conf["authMethod"] != "http" || conf["rtmp"] != false || conf["hls"] != false || conf["api"] != false || conf["webrtc"] != false || conf["moq"] != false {
		t.Fatal("unreviewed server enabled")
	}
	if strings.Contains(string(b), "runOn") || strings.Contains(string(b), "authInternalUsers") {
		t.Fatal("unsafe hooks or anonymous auth")
	}
}
func TestSegmentServingRejectsTraversalSymlinksAndUnreadyOutput(t *testing.T) {
	s, source := fixture(t)
	source.active.Store(true)
	for _, name := range []string{"../outputs.json", "/etc/passwd", "index.mpd/../file", "secret.txt"} {
		w := httptest.NewRecorder()
		s.ServeSegment(w, httptest.NewRequest("GET", "/", nil), "synthetic", name)
		if w.Code == 200 {
			t.Fatal("unsafe segment")
		}
	}
	s.cancel()
	<-s.done
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "index.mpd"), []byte("<MPD/>"), 0600)
	s.mu.Lock()
	s.jobs["synthetic"] = &worker{dir: dir, dashState: "running"}
	s.mu.Unlock()
	defer func() { s.mu.Lock(); delete(s.jobs, "synthetic"); s.mu.Unlock() }()
	w := httptest.NewRecorder()
	s.ServeSegment(w, httptest.NewRequest("GET", "/", nil), "synthetic", "index.mpd")
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/dash+xml" {
		t.Fatal("manifest unavailable")
	}
	source.allowed.Store(false)
	w = httptest.NewRecorder()
	s.ServeSegment(w, httptest.NewRequest("GET", "/", nil), "synthetic", "index.mpd")
	if w.Code == 200 {
		t.Fatal("grant bypass")
	}
}
func TestCloseReleasesPrivateRuntime(t *testing.T) {
	source := &testSource{}
	s, e := New(context.Background(), filepath.Join(t.TempDir(), "outputs"), source, testKeys{})
	if e != nil {
		t.Fatal(e)
	}
	root := s.runtime
	done := make(chan struct{})
	go func() { s.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("close blocked")
	}
	if _, e = os.Stat(root); !os.IsNotExist(e) {
		t.Fatal("runtime leaked")
	}
}
