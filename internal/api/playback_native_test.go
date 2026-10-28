package api

import (
	"bytes"
	"context"
	"golang.org/x/crypto/bcrypt"
	"io"
	"kore-edge/internal/accounts"
	"kore-edge/internal/hls"
	"kore-edge/internal/live"
	"kore-edge/internal/mediaresources"
	"kore-edge/internal/playback"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type nativeVLCCatalog struct {
	vlcCatalog
	tool  string
	delay time.Duration
}
type nativeVLCInput struct {
	io.ReadCloser
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func (p *nativeVLCInput) Close() error {
	p.once.Do(func() { p.cancel(); p.ReadCloser.Close(); <-p.done })
	return nil
}
func (c *nativeVLCCatalog) OpenHTTP(parent context.Context, _ string) (io.ReadCloser, error) {
	c.opens.Add(1)
	if c.delay > 0 {
		select {
		case <-parent.Done():
			return nil, parent.Err()
		case <-time.After(c.delay):
		}
	}
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, c.tool, hls.SyntheticArgs(true)...)
	mediaresources.Configure(cmd)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	r, e := cmd.StdoutPipe()
	if e != nil {
		cancel()
		return nil, e
	}
	if e = cmd.Start(); e != nil {
		cancel()
		r.Close()
		return nil, e
	}
	p := &nativeVLCInput{ReadCloser: r, cancel: cancel, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done) }()
	return p, nil
}

func TestVLCNativeHLSPlaylistAndSegmentWithoutCookies(t *testing.T) {
	tool := os.Getenv("KORE_PLAYBACK_FFMPEG")
	if tool == "" {
		t.Skip("portable FFmpeg fixture not requested")
	}
	probe := filepath.Join(filepath.Dir(tool), "ffprobe")
	if strings.HasSuffix(tool, ".exe") {
		probe += ".exe"
	}
	if _, e := os.Stat(probe); e != nil {
		t.Fatal("native test requires ffprobe alongside FFmpeg")
	}
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
	c := &nativeVLCCatalog{tool: tool, delay: 4 * time.Second}
	c.allowed.Store(true)
	m, e := live.New(context.Background(), filepath.Join(dir, "live"), c)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if e = m.Save(live.ChannelConfig{ChannelID: "synthetic", Protocol: "http_ts", HLS: true}, live.Tools{FFmpeg: tool, FFprobe: probe}, 0); e != nil {
		t.Fatal(e)
	}
	if e = m.Start("synthetic"); e != nil {
		t.Fatal(e)
	}
	a := &API{Accounts: users, Playback: store, Live: m, PublicOrigin: "https://edge.example"}
	server := httptest.NewTLSServer(a.Handler())
	defer server.Close()
	client := server.Client()
	client.Timeout = 4 * time.Second
	raw, e := store.Permanent("synthetic")
	if e != nil {
		t.Fatal("issue")
	}
	playlistURL := server.URL + "/api/v1/playback/" + raw + "/synthetic/index.m3u8"
	var playlist []byte
	deadline := time.Now().Add(24 * time.Second)
	for time.Now().Before(deadline) {
		resp, e := client.Get(playlistURL)
		if e == nil {
			data, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
			resp.Body.Close()
			if resp.StatusCode == 200 {
				playlist = data
				break
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !bytes.HasPrefix(playlist, []byte("#EXTM3U")) {
		t.Fatal("synthetic HLS never ready", m.Snapshot().Streams)
	}
	var segment string
	for _, line := range strings.Split(string(playlist), "\n") {
		if playbackSegment.MatchString(strings.TrimSpace(line)) {
			segment = strings.TrimSpace(line)
			break
		}
	}
	if segment == "" {
		t.Fatal("playlist has no local segment")
	}
	base, _ := url.Parse(playlistURL)
	ref, _ := url.Parse(segment)
	resp, e := client.Get(base.ResolveReference(ref).String())
	if e != nil {
		t.Fatal("segment request")
	}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8*1024*1024))
	resp.Body.Close()
	if resp.StatusCode != 200 || len(data) < 188 || data[0] != 0x47 {
		t.Fatal("VLC segment rejected", resp.StatusCode)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, probe, "-v", "error", "-f", "mpegts", "-show_entries", "stream=codec_type,codec_name", "-of", "json", "pipe:0")
	mediaresources.Configure(cmd)
	cmd.Stdin = bytes.NewReader(data)
	info, e := cmd.Output()
	if e != nil || hls.ValidateCodecs(info) != nil {
		t.Fatal("not compatible H.264/AAC TS")
	}

	ts, e := client.Get(strings.TrimSuffix(playlistURL, "index.m3u8") + "stream.ts")
	if e != nil {
		t.Fatal("permanent TS request")
	}
	tsData := make([]byte, 188*400)
	_, e = io.ReadFull(ts.Body, tsData)
	ts.Body.Close()
	if e != nil || ts.StatusCode != 200 {
		t.Fatal("permanent TS output", ts.StatusCode, e)
	}
	if hls.ValidateInput(ctx, hls.Config{Enabled: true, FFmpeg: tool, FFprobe: probe, Root: filepath.Join(dir, "codec-check")}, tsData) != nil {
		t.Fatal("permanent TS does not preserve H.264/AAC")
	}
	if c.opens.Load() != 1 {
		t.Fatal("HLS player opened duplicate origin")
	}
}
