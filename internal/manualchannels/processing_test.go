package manualchannels

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"kore-edge/internal/hls"
	"kore-edge/internal/live"
	"kore-edge/internal/mediaresources"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type observedInput struct {
	io.ReadCloser
	once   sync.Once
	closed func()
}

func (r *observedInput) Close() error { err := r.ReadCloser.Close(); r.once.Do(r.closed); return err }

func TestLiveSupervisorRecoversToBackupWithoutParallelPulls(t *testing.T) {
	s, id := processingFixture(t)
	inputs := []Input{{Key: "primary", Protocol: "http_ts"}, {URL: "https://backup.example/live.ts", Protocol: "http_ts"}}
	if e := s.SaveProcessing(id, &inputs, nil, s.Revision()); e != nil {
		t.Fatal(e)
	}
	var open, peak, primary, backup atomic.Int32
	s.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		count := open.Add(1)
		for old := peak.Load(); count > old && !peak.CompareAndSwap(old, count); old = peak.Load() {
		}
		var body io.ReadCloser
		if r.URL.Host == "primary.example" {
			primary.Add(1)
			body = io.NopCloser(bytes.NewReader(bytes.Repeat(packet(), 7)))
		} else {
			backup.Add(1)
			reader, writer := io.Pipe()
			body = reader
			go func() {
				defer writer.Close()
				ticker := time.NewTicker(5 * time.Millisecond)
				defer ticker.Stop()
				for {
					select {
					case <-r.Context().Done():
						return
					case <-ticker.C:
						if _, e := writer.Write(bytes.Repeat(packet(), 7)); e != nil {
							return
						}
					}
				}
			}()
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/mp2t"}}, Body: &observedInput{ReadCloser: body, closed: func() { open.Add(-1) }}}, nil
	})}
	manager, e := live.New(context.Background(), filepath.Join(t.TempDir(), "live"), s)
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	if manager.Save(live.ChannelConfig{ChannelID: id, Protocol: "http_ts", Resume: true}, live.Tools{}, 0) != nil || manager.Start(id) != nil {
		t.Fatal("start")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		v := manager.Snapshot()
		if backup.Load() == 1 && len(v.Streams) == 1 && v.Streams[0].State == "running" {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if backup.Load() != 1 || primary.Load() != 1 || peak.Load() != 1 {
		t.Fatal("failover not isolated", primary.Load(), backup.Load(), peak.Load())
	}
	view, _ := s.Processing(id)
	if view.ActiveKey != view.Inputs[1].Key {
		t.Fatal("backup not selected")
	}
	if e = manager.Stop(id); e != nil {
		t.Fatal(e)
	}
	if open.Load() != 0 {
		t.Fatal("manual stop leaked an origin")
	}
}

func processingFixture(t *testing.T) (*Service, string) {
	t.Helper()
	s := fixture(t)
	id, e := s.Save(Source{Name: "Synthetic", URL: "https://synthetic-user:synthetic-password@primary.example/live/mpegts?token=synthetic-token", Enabled: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	return s, id
}

func TestProcessingPersistenceSecretBoundaryRevisionAndLegacyRollback(t *testing.T) {
	s, id := processingFixture(t)
	before, _ := os.ReadFile(filepath.Join(s.directory, "channels.json"))
	initial, e := s.Processing(id)
	if e != nil || initial.Transcode.Mode != "copy" || len(initial.Inputs) != 1 {
		t.Fatal("initial profile", e)
	}
	inputs := []Input{{Key: "primary", Protocol: "http_ts"}, {URL: "https://backup-user:backup-password@backup.example/stream.m3u8?token=backup-token", Protocol: "hls"}}
	if e = s.SaveProcessing(id, &inputs, nil, 1); e != nil {
		t.Fatal(e)
	}
	view, _ := s.Processing(id)
	b, _ := json.Marshal(view)
	for _, secret := range []string{"synthetic-password", "synthetic-token", "backup-password", "backup-token"} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatal("inventory exposed credentials")
		}
	}
	if view.Revision != 2 || len(view.Inputs) != 2 || view.Inputs[1].Key == "" {
		t.Fatal("missing saved configuration")
	}
	if e = s.SaveProcessing(id, &inputs, nil, 1); !errors.Is(e, ErrConflict) {
		t.Fatal("stale write accepted")
	}
	after, _ := os.ReadFile(filepath.Join(s.directory, "channels.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("legacy origin mutated")
	}
	copy, e := New(s.directory, nil)
	if e != nil {
		t.Fatal(e)
	}
	defer copy.Close()
	restored, e := copy.Processing(id)
	if e != nil || len(restored.Inputs) != 2 {
		t.Fatal("profile not restored", e)
	}
	reversed := []Input{{Key: restored.Inputs[1].Key, Protocol: "hls"}, {Key: "primary", Protocol: "http_ts"}}
	if e = copy.SaveProcessing(id, &reversed, nil, restored.Revision); e != nil {
		t.Fatal(e)
	}
	p := copy.processing.Entries[id]
	if !strings.Contains(p.Inputs[0].URL, "backup-password") || !strings.Contains(p.Inputs[1].URL, "synthetic-password") {
		t.Fatal("reorder did not preserve credentials by stable key")
	}
	// A legacy editor changing the origin wins over an obsolete profile.
	if _, e = copy.Save(Source{ID: id, Name: "Synthetic", URL: "https://replacement.example/new/mpegts", Enabled: true}, copy.Revision()); e != nil {
		t.Fatal(e)
	}
	v, _ := copy.Processing(id)
	if len(v.Inputs) != 1 || !strings.Contains(v.Inputs[0].Address, "replacement.example") {
		t.Fatal("obsolete profile masked legacy edit")
	}
	if runtime.GOOS != "windows" {
		file := filepath.Join(s.directory, "processing.json")
		info, _ := os.Stat(file)
		if info.Mode().Perm() != 0600 {
			t.Fatal("profile permissions")
		}
		if os.Chmod(file, 0644) != nil {
			t.Fatal("chmod")
		}
		if _, e = New(s.directory, nil); e == nil {
			t.Fatal("public credentials accepted")
		}
	}
}

func TestSequentialFailoverUsesOneOriginAndDefaultDoesNotSpawnEncoder(t *testing.T) {
	s, id := processingFixture(t)
	inputs := []Input{{Key: "primary", Protocol: "http_ts"}, {URL: "https://backup.example/live/mpegts", Protocol: "http_ts"}}
	if e := s.SaveProcessing(id, &inputs, nil, s.Revision()); e != nil {
		t.Fatal(e)
	}
	var hosts []string
	s.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		hosts = append(hosts, r.URL.Host)
		if r.URL.Host == "primary.example" {
			return nil, ErrUnavailable
		}
		return response(bytes.Repeat(packet(), 7), "video/mp2t"), nil
	})}
	s.command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("copy mode launched an encoder")
		return nil
	}
	reader, handled, e := s.OpenConfigured(context.Background(), id, "http_ts", "")
	if e != nil || !handled {
		t.Fatal("failover failed", e)
	}
	data, e := io.ReadAll(reader)
	reader.Close()
	if e != nil || len(data) != 1316 || strings.Join(hosts, ",") != "primary.example,backup.example" {
		t.Fatal("invalid source sequence", hosts, e)
	}
	v, _ := s.Processing(id)
	if v.ActiveKey != v.Inputs[1].Key {
		t.Fatal("active origin missing")
	}
	// The next attempt tries the next priority after the last opened origin.
	hosts = nil
	reader, _, e = s.OpenConfigured(context.Background(), id, "http_ts", "")
	if e != nil {
		t.Fatal(e)
	}
	reader.Close()
	if len(hosts) != 2 {
		t.Fatal("retry rotation failed")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	hosts = nil
	if _, _, e = s.OpenConfigured(ctx, id, "http_ts", ""); e == nil || len(hosts) != 0 {
		t.Fatal("cancelled attempt opened an origin")
	}
}

func TestProcessingRejectsUnsafeOrDuplicateInputsAndInvalidTranscode(t *testing.T) {
	s, id := processingFixture(t)
	cases := [][]Input{{}, {{Key: "unknown", Protocol: "http_ts"}}, {{Key: "primary", Protocol: "http_ts"}, {Key: "primary", Protocol: "http_ts"}}, {{URL: "file:///etc/passwd", Protocol: "http_ts"}}, {{URL: "http://127.0.0.1:8080/x.ts", Protocol: "http_ts"}}, {{URL: "udp://239.1.1.1:5000", Protocol: "udp"}}}
	for _, v := range cases {
		if e := s.SaveProcessing(id, &v, nil, s.Revision()); e == nil {
			t.Fatal("invalid input accepted")
		}
	}
	valid := Transcode{Mode: "h264", Device: "cpu", VideoKbps: 1000, Width: 640, Height: 360, FPS: 25, Audio: "aac", AudioKbps: 96}
	if !validateTranscode(valid) {
		t.Fatal("valid settings rejected")
	}
	for _, change := range []func(*Transcode){func(v *Transcode) { v.Device = "cpu -i /etc/passwd" }, func(v *Transcode) { v.Device = "vaapi:../../etc/passwd" }, func(v *Transcode) { v.Width = 641 }, func(v *Transcode) { v.VideoKbps = 0 }, func(v *Transcode) { v.FPS = 120 }, func(v *Transcode) { v.Audio = "invalid" }} {
		v := valid
		change(&v)
		if e := s.SaveProcessing(id, nil, &v, s.Revision()); e == nil {
			t.Fatal("invalid processing accepted")
		}
	}
	if e := s.SaveProcessing(id, nil, &valid, s.Revision()); e != nil {
		t.Fatal(e)
	}
	args := strings.Join(transcodeArgs(valid), " ")
	if strings.Contains(args, "primary.example") || !strings.Contains(args, "-i pipe:0") || !strings.Contains(args, "libx264") {
		t.Fatal("unsafe encoder arguments")
	}
	copied := Transcode{Mode: "copy"}
	if e := s.SaveProcessing(id, nil, &copied, s.Revision()); e != nil {
		t.Fatal(e)
	}
	if _, handled, e := s.OpenConfigured(context.Background(), "local_"+strings.Repeat("b", 32), "http_ts", ""); handled || e != nil {
		t.Fatal("missing profile should use legacy authorization")
	}
}

func TestNativeTranscodeChangesResolutionAndReleasesProcess(t *testing.T) {
	tool := os.Getenv("KORE_MANUAL_FFMPEG")
	if tool == "" {
		t.Skip("set KORE_MANUAL_FFMPEG for synthetic native encoder validation")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	sample, e := exec.CommandContext(ctx, tool, hls.SyntheticArgs(false)...).Output()
	if e != nil {
		t.Fatal("synthetic generation", e)
	}
	s, _ := processingFixture(t)
	cap := s.Encoders(ctx, tool)
	if len(cap.Devices) == 0 || !s.CanEncode(ctx, tool, "cpu") {
		t.Fatal("CPU encoder probe failed")
	}
	t.Log("synthetic CPU encoder available; GPU availability depends on runner hardware")
	config := Transcode{Mode: "h264", Device: "cpu", VideoKbps: 800, Width: 640, Height: 360, FPS: 25, Audio: "aac", AudioKbps: 96}
	input := io.NopCloser(bytes.NewReader(sample))
	reader, e := s.transcode(ctx, input, tool, config)
	if e != nil {
		t.Fatal(e)
	}
	data, e := io.ReadAll(io.LimitReader(reader, 8*1024*1024))
	reader.Close()
	if e != nil {
		t.Fatal("encoded output", e)
	}
	if len(data) < 188*20 || data[0] != 0x47 {
		t.Fatal("missing encoded TS")
	}
	probe := filepath.Join(filepath.Dir(tool), "ffprobe")
	if runtime.GOOS == "windows" {
		probe += ".exe"
	}
	cmd := exec.CommandContext(ctx, probe, "-v", "quiet", "-select_streams", "v:0", "-show_entries", "stream=codec_name,width,height", "-of", "json", "-i", "pipe:0")
	cmd.Stdin = bytes.NewReader(data)
	out, e := cmd.Output()
	if e != nil {
		t.Fatal("probe encoded output", e)
	}
	var result struct {
		Streams []struct {
			Codec  string `json:"codec_name"`
			Width  int    `json:"width"`
			Height int    `json:"height"`
		} `json:"streams"`
	}
	if json.Unmarshal(out, &result) != nil || len(result.Streams) == 0 || result.Streams[0].Width != 640 || result.Streams[0].Height != 360 || result.Streams[0].Codec != "h264" {
		t.Fatal("profile not applied")
	}
	if len(s.transcodeSlots) != 0 {
		t.Fatal("encoder slot leaked")
	}
	// Saturation closes the source without spawning a third encoder.
	s.transcodeSlots <- struct{}{}
	s.transcodeSlots <- struct{}{}
	if _, e = s.transcode(ctx, io.NopCloser(bytes.NewReader(sample)), tool, config); !errors.Is(e, mediaresources.ErrBusy) {
		t.Fatal("unbounded encoder admission")
	}
	<-s.transcodeSlots
	<-s.transcodeSlots
	// Cancel while output is unread: both child process and pipes must terminate.
	child, stop := context.WithCancel(ctx)
	reader, e = s.transcode(child, io.NopCloser(bytes.NewReader(sample)), tool, config)
	if e != nil {
		t.Fatal(e)
	}
	stop()
	done := make(chan struct{})
	go func() { reader.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("encoder cancellation blocked")
	}
}
