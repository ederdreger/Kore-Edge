package hls_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kore-edge/internal/hls"
	"kore-edge/internal/stream"
)

func TestFFmpegIntegration(t *testing.T) {
	ffmpeg := os.Getenv("KORE_EDGE_FFMPEG")
	ffprobe := os.Getenv("KORE_EDGE_FFPROBE")
	if ffmpeg == "" || ffprobe == "" {
		t.Skip("set KORE_EDGE_FFMPEG and KORE_EDGE_FFPROBE for real AV integration")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	var allowed atomic.Bool
	allowed.Store(true)
	allow := func(id string, _ time.Time) bool { return id == "test" && allowed.Load() }
	root := t.TempDir()
	m, e := hls.New(ctx, hls.Config{Enabled: true, FFmpeg: ffmpeg, FFprobe: ffprobe, Root: root}, allow)
	if e != nil {
		t.Fatal("codec preflight failed", e)
	}
	defer m.Close()
	engine, e := stream.NewWithOutput(ctx, stream.AVFactory(ffmpeg), allow, stream.DefaultPolicy(), func(ctx context.Context, id string) (stream.Output, error) { return m.Open(ctx, id) })
	if e != nil {
		t.Fatal(e)
	}
	defer engine.Close()
	if engine.Start("test") != nil {
		t.Fatal("start")
	}
	server := httptest.NewServer(m)
	defer server.Close()
	client := &http.Client{Timeout: 2 * time.Second}
	playlistURL := server.URL + "/hls/test/index.m3u8"
	get := func(url string) (int, string) {
		resp, e := client.Get(url)
		if e != nil {
			return 0, ""
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
		return resp.StatusCode, string(b)
	}
	var first string
	deadline := time.NewTimer(30 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for first == "" {
		code, body := get(playlistURL)
		if code == 200 {
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, "seg") && strings.HasSuffix(line, ".ts") {
					first = line
					break
				}
			}
		}
		select {
		case <-deadline.C:
			t.Fatalf("no playable playlist: %+v", engine.Snapshot())
		case <-ticker.C:
		}
	}
	if code, _ := get(server.URL + "/hls/test/" + first); code != 200 {
		t.Fatal("segment unavailable")
	}
	status := engine.Snapshot()
	if len(status) != 1 || status[0].Transport.PCRCount < 2 || status[0].Transport.PCRIntervalSeconds == nil || status[0].Transport.CCState != "measured" || status[0].Transport.CCErrors != 0 || status[0].Transport.TEI != 0 {
		t.Fatal("AV transport metrics unavailable")
	}
	// Actually decode both audio and video from the HTTP HLS endpoint.
	decodeCtx, stop := context.WithTimeout(ctx, 12*time.Second)
	cmd := exec.CommandContext(decodeCtx, ffmpeg, "-v", "error", "-nostdin", "-rw_timeout", "5000000", "-i", playlistURL, "-t", "3", "-map", "0:v:0", "-map", "0:a:0", "-progress", "pipe:1", "-f", "null", "-")
	output, e := cmd.Output()
	stop()
	if e != nil || !strings.Contains(string(output), "progress=end") {
		t.Fatal("HLS audio/video decode failed", e)
	}
	// A real player runs with dummy SDL sinks, so validation opens no desktop window.
	ffplay := os.Getenv("KORE_EDGE_FFPLAY")
	if ffplay != "" {
		playCtx, stop := context.WithTimeout(ctx, 6*time.Second)
		player := exec.CommandContext(playCtx, ffplay, "-loglevel", "info", "-stats", "-nodisp", "-analyzeduration", "1000000", "-probesize", "1000000", playlistURL)
		player.Env = append(os.Environ(), "SDL_VIDEODRIVER=dummy", "SDL_AUDIODRIVER=dummy", "SDL_RENDER_DRIVER=software")
		playerOutput, _ := player.CombinedOutput()
		stop()
		// A live playlist has no EOF: deliberately stop the player after a
		// bounded observation window and verify its playback clock advanced.
		matches := regexp.MustCompile(`([0-9]+\.[0-9]+) M-[AV]:`).FindAllStringSubmatch(string(playerOutput), -1)
		if len(matches) < 2 {
			t.Fatal("FFplay did not establish playback")
		}
		firstClock, _ := strconv.ParseFloat(matches[0][1], 64)
		lastClock, _ := strconv.ParseFloat(matches[len(matches)-1][1], 64)
		if lastClock-firstClock < 2 {
			t.Fatal("FFplay playback clock did not advance")
		}
		t.Log("FFplay audio playback clock advanced with dummy SDL sink; live player was intentionally stopped")
	}
	expired := false
	for !expired {
		code, _ := get(server.URL + "/hls/test/" + first)
		if code == 404 {
			expired = true
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("old segment did not expire")
		case <-ticker.C:
		}
	}
	t.Log("H.264/AAC HTTP decode passed; old segment expired")
	allowed.Store(false)
	if code, _ := get(playlistURL); code != 403 {
		t.Fatal("expired authorization not enforced")
	}
	engine.Close()
	m.Close()
	entries, e := os.ReadDir(root)
	if e != nil || len(entries) != 0 {
		t.Fatal("temporary files leaked")
	}
}
