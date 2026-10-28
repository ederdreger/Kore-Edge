package output

import (
	"bytes"
	"context"
	"io"
	"kore-edge/internal/hls"
	"kore-edge/internal/mediaresources"
	"net/http"
	"os"

	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var segmentName = regexp.MustCompile(`^(index\.mpd|master\.m3u8|media_[0-9]{1,3}\.m3u8|init-[0-9]{1,3}\.m4s|chunk-[0-9]{1,3}-[0-9]{5,20}\.m4s)$`)

func dashArgs(c Channel) []string {
	return []string{"-hide_banner", "-loglevel", "quiet", "-nostdin", "-protocol_whitelist", "pipe,file", "-f", "mpegts", "-analyzeduration", "1000000", "-probesize", "1000000", "-i", "pipe:0", "-map", "0:v:0", "-map", "0:a:0", "-c", "copy", "-tag:v", "avc1", "-tag:a", "mp4a", "-f", "dash", "-seg_duration", strconv.Itoa(c.Duration), "-window_size", strconv.Itoa(c.Segments), "-extra_window_size", "2", "-remove_at_exit", "1", "-use_template", "1", "-use_timeline", "1", "-init_seg_name", "init-$RepresentationID$.m4s", "-media_seg_name", "chunk-$RepresentationID$-$Number%05d$.m4s", "-hls_playlist", "1", "-hls_master_name", "master.m3u8", "index.mpd"}
}
func (s *Service) packageStream(id string, j *worker) {
	defer func() {
		j.mu.Lock()
		dir := j.dir
		j.dir = ""
		j.mu.Unlock()
		if dir != "" {
			os.RemoveAll(dir)
		}
	}()
	for {
		if j.ctx.Err() != nil {
			return
		}
		j.mu.Lock()
		j.dashState = "starting"
		j.mu.Unlock()
		release, ok := mediaresources.Output()
		if !ok {
			j.mu.Lock()
			j.dashState = "waiting_resources"
			j.mu.Unlock()
		} else {
			s.packageAttempt(id, j)
			release()
		}
		select {
		case <-j.ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}
func (s *Service) packageAttempt(id string, j *worker) {
	set := func(v string) { j.mu.Lock(); j.dashState = v; j.mu.Unlock() }
	if s.ffmpeg == "" || s.ffprobe == "" {
		set("tool_unavailable")
		return
	}
	if !s.source.PlaybackAllowed(id, true) {
		set("unauthorized")
		return
	}
	ctx, cancel := context.WithCancel(j.ctx)
	defer cancel()
	input, e := s.source.SharedStream(ctx, id)
	if e != nil {
		set("waiting_signal")
		return
	}
	defer input.Close()
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
			input.Close()
		}
	}()
	defer func() { cancel(); <-done }()
	// Bound the sample in both size and wall time; no extra connection to the origin.
	sampleDone := make(chan struct{})
	var sample []byte
	var sampleError error
	go func() { sample, sampleError = io.ReadAll(io.LimitReader(input, 128*1024)); close(sampleDone) }()
	select {
	case <-sampleDone:
	case <-time.After(6 * time.Second):
		cancel()
		<-sampleDone
		set("waiting_signal")
		return
	case <-ctx.Done():
		<-sampleDone
		return
	}
	if sampleError != nil || len(sample) < 188 {
		set("waiting_signal")
		return
	}
	config := hls.Config{Enabled: true, FFmpeg: s.ffmpeg, FFprobe: s.ffprobe, Root: s.runtime}
	if hls.ValidateInput(ctx, config, sample) != nil {
		set("codec_or_tools_unavailable")
		return
	}
	dir, e := os.MkdirTemp(s.runtime, "dash-")
	if e != nil {
		set("storage_unavailable")
		return
	}
	defer os.RemoveAll(dir)
	j.mu.Lock()
	j.dir = dir
	j.mu.Unlock()
	defer func() {
		j.mu.Lock()
		if j.dir == dir {
			j.dir = ""
		}
		j.mu.Unlock()
	}()
	cmd := s.command(ctx, s.ffmpeg, dashArgs(j.config)...)
	mediaresources.Configure(cmd)
	cmd.Dir = dir
	cmd.Stdin = io.MultiReader(bytes.NewReader(sample), input)
	cmd.Stdout = io.Discard
	if cmd.Stderr == nil {
		cmd.Stderr = io.Discard
	}
	cmd.WaitDelay = time.Second
	if cmd.Start() != nil {
		set("failed")
		return
	}
	exited := make(chan struct{})
	go func() { _ = cmd.Wait(); cancel(); close(exited) }()
	defer func() { cancel(); input.Close(); <-exited }()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			set("reconnecting")
			return
		case <-ticker.C:
			if !s.source.PlaybackAllowed(id, true) || !s.source.IsActive(id) {
				set("unauthorized")
				return
			}
			entries, e := os.ReadDir(dir)
			if e != nil {
				set("storage_unavailable")
				return
			}
			var total int64
			for _, entry := range entries {
				info, e := entry.Info()
				if e != nil || !info.Mode().IsRegular() {
					set("storage_unavailable")
					return
				}
				total += info.Size()
			}
			if total > 128*1024*1024 || len(entries) > 100 {
				set("storage_limit")
				return
			}
			info, e := os.Stat(filepath.Join(dir, "index.mpd"))
			if e == nil && info.Size() > 0 && time.Since(info.ModTime()) < 15*time.Second {
				set("running")
			} else {
				set("waiting_signal")
			}
		}
	}
}
func (s *Service) ServeSegment(w http.ResponseWriter, r *http.Request, id, name string) {
	select {
	case s.requests <- struct{}{}:
		defer func() { <-s.requests }()
	default:
		w.WriteHeader(429)
		return
	}
	if !safeID.MatchString(id) || !segmentName.MatchString(name) || !s.source.PlaybackAllowed(id, true) || !s.source.IsActive(id) {
		w.WriteHeader(403)
		return
	}
	s.mu.Lock()
	j := s.jobs[id]
	s.mu.Unlock()
	if j == nil {
		w.WriteHeader(503)
		return
	}
	j.mu.Lock()
	dir, ready := j.dir, j.dashState == "running"
	j.mu.Unlock()
	if !ready || dir == "" {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(503)
		return
	}
	path := filepath.Join(dir, name)
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() > 8*1024*1024 {
		w.WriteHeader(404)
		return
	}
	f, e := os.Open(path)
	if e != nil {
		w.WriteHeader(404)
		return
	}
	defer f.Close()
	data, e := io.ReadAll(io.LimitReader(f, 8*1024*1024+1))
	if e != nil || len(data) > 8*1024*1024 {
		w.WriteHeader(404)
		return
	}
	mime := "video/iso.segment"
	if strings.HasSuffix(name, ".mpd") {
		mime = "application/dash+xml"
	}
	if strings.HasSuffix(name, ".m3u8") {
		mime = "application/vnd.apple.mpegurl"
	}
	w.Header().Set("Content-Type", mime)
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != "HEAD" {
		w.Write(data)
	}
}
