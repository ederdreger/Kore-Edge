package output

import (
	"context"
	"encoding/json"
	"io"
	"kore-edge/internal/edgecatalog"
	"kore-edge/internal/hls"
	"kore-edge/internal/live"
	"kore-edge/internal/mediaresources"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type nativeCatalog struct {
	tool    string
	opens   atomic.Int32
	allowed atomic.Bool
}

func (c *nativeCatalog) Snapshot() edgecatalog.Snapshot {
	return edgecatalog.Snapshot{Available: true, Catalog: &edgecatalog.Manifest{Version: 1, Channels: []edgecatalog.Channel{{ID: "synthetic", Name: "Synthetic", Outputs: []string{"hls", "udp"}, Protocols: []edgecatalog.Protocol{{ID: "http_ts"}}}}}}
}
func (c *nativeCatalog) Authorized(string, string) bool       { return c.allowed.Load() }
func (c *nativeCatalog) OutputAuthorized(string, string) bool { return c.allowed.Load() }
func (c *nativeCatalog) OpenHLS(context.Context, string, string) (io.ReadCloser, error) {
	return nil, live.ErrDenied
}
func (c *nativeCatalog) OpenSRT(context.Context, string) (io.ReadCloser, error) {
	return nil, live.ErrDenied
}

type nativeInput struct {
	io.ReadCloser
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func (r *nativeInput) Close() error {
	r.once.Do(func() { r.cancel(); r.ReadCloser.Close(); <-r.done })
	return nil
}
func (c *nativeCatalog) OpenHTTP(parent context.Context, id string) (io.ReadCloser, error) {
	c.opens.Add(1)
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, c.tool, hls.SyntheticArgs(true)...)
	mediaresources.Configure(cmd)
	cmd.Stderr = io.Discard
	out, e := cmd.StdoutPipe()
	if e != nil {
		cancel()
		return nil, e
	}
	if e = cmd.Start(); e != nil {
		cancel()
		return nil, e
	}
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	return &nativeInput{ReadCloser: out, cancel: cancel, done: done}, nil
}
func port(t *testing.T, udp bool) int {
	t.Helper()
	if udp {
		l, e := net.ListenPacket("udp4", "127.0.0.1:0")
		if e != nil {
			t.Fatal(e)
		}
		p := l.LocalAddr().(*net.UDPAddr).Port
		l.Close()
		return p
	}
	l, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	p := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return p
}

func TestNativeOutputsDASHRTSPRTMPAndEncryptedSRTShareOneSyntheticOrigin(t *testing.T) {
	ffmpeg, mtx := os.Getenv("KORE_OUTPUT_FFMPEG"), os.Getenv("KORE_OUTPUT_MEDIAMTX")
	if ffmpeg == "" || mtx == "" || runtime.GOOS != "linux" {
		t.Skip("set pinned KORE_OUTPUT_FFMPEG and KORE_OUTPUT_MEDIAMTX on Linux for real protocol tests")
	}
	probe := filepath.Join(filepath.Dir(ffmpeg), "ffprobe")
	ctx, cancel := context.WithTimeout(context.Background(), 65*time.Second)
	defer cancel()
	catalog := &nativeCatalog{tool: ffmpeg}
	catalog.allowed.Store(true)
	manager, e := live.New(ctx, filepath.Join(t.TempDir(), "live"), catalog)
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	if e = manager.Save(live.ChannelConfig{ChannelID: "synthetic", Protocol: "http_ts", HLS: true, Resume: true}, live.Tools{FFmpeg: ffmpeg, FFprobe: probe}, 0); e != nil {
		t.Fatal(e)
	}
	if manager.Start("synthetic") != nil {
		t.Fatal("start")
	}
	s, e := New(ctx, filepath.Join(t.TempDir(), "outputs"), manager, testKeys{})
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	s.mu.Lock()
	s.mtx = mtx
	s.ffmpeg = ffmpeg
	s.ffprobe = probe
	s.mu.Unlock()
	n := Network{Enabled: true, BindIP: "127.0.0.1", PublicHost: "127.0.0.1", RTSP: port(t, false), RTMP: port(t, false), SRT: port(t, true)}
	if n.RTMP == n.RTSP {
		n.RTMP = port(t, false)
	}
	c := DefaultChannel()
	c.DASH = true
	c.RTSP = true
	c.RTMP = true
	c.SRT = true
	if e = s.Save("synthetic", c, &n, 0); e != nil {
		t.Fatal("save", e)
	}
	deadline := time.Now().Add(22 * time.Second)
	ready := false
	for time.Now().Before(deadline) {
		v := s.Snapshot("synthetic")
		if v.Status.Feed == "sending" && v.Status.DASH == "running" {
			ready = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !ready {
		v := s.Snapshot("synthetic")
		t.Fatalf("outputs not ready: bridge=%s feed=%s DASH=%s", v.Status.Bridge, v.Status.Feed, v.Status.DASH)
	}
	links := s.Links("synthetic")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.ServeSegment(w, r, "synthetic", strings.TrimPrefix(r.URL.Path, "/"))
	}))
	defer server.Close()
	links["dash"] = server.URL + "/index.mpd"
	for _, protocol := range []string{"dash", "rtsp", "rtmp", "srt"} {
		child, stop := context.WithTimeout(ctx, 8*time.Second)
		args := []string{"-v", "error", "-analyzeduration", "1000000", "-probesize", "1000000"}
		if protocol == "rtsp" {
			args = append(args, "-rtsp_transport", "tcp")
		}
		args = append(args, "-i", links[protocol], "-show_entries", "stream=codec_name", "-of", "json")
		cmd := exec.CommandContext(child, probe, args...)
		data, err := cmd.Output()
		stop()
		if err != nil {
			t.Fatalf("%s could not be read: %v", protocol, err)
		}
		var result struct {
			Streams []struct {
				Codec string `json:"codec_name"`
			} `json:"streams"`
		}
		if json.Unmarshal(data, &result) != nil {
			t.Fatal("invalid probe response")
		}
		video, audio := false, false
		for _, stream := range result.Streams {
			video = video || stream.Codec == "h264"
			audio = audio || stream.Codec == "aac"
		}
		if !video || !audio {
			t.Fatal(protocol, "lost expected synthetic tracks")
		}
		t.Log(protocol + " decoded H.264/AAC")
	}
	// The same MediaMTX path cannot be used to bypass a disabled output protocol.
	c.RTMP = false
	if e = s.Save("synthetic", c, nil, s.Snapshot("synthetic").Revision); e != nil {
		t.Fatal(e)
	}
	time.Sleep(time.Second)
	child, stop := context.WithTimeout(ctx, 4*time.Second)
	cmd := exec.CommandContext(child, probe, "-v", "quiet", "-i", links["rtmp"], "-show_streams")
	_, err := cmd.Output()
	stop()
	if err == nil {
		t.Fatal("disabled RTMP remained readable")
	}
	if catalog.opens.Load() != 1 {
		t.Fatal("outputs opened duplicate origins", catalog.opens.Load())
	}
	if manager.Stop("synthetic") != nil {
		t.Fatal("stop")
	}
	deadline = time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		conn, e := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(n.RTSP)), 100*time.Millisecond)
		if e != nil {
			return
		}
		conn.Close()
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("listener remained after all channels stopped")
}
