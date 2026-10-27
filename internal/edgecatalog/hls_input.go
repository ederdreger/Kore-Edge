package edgecatalog

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"kore-edge/internal/mediaresources"
	"net"
	"net/http"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

type processInput struct {
	output io.ReadCloser
	cancel context.CancelFunc
	server *http.Server
	done   chan struct{}
	once   sync.Once
}

func (p *processInput) Read(b []byte) (int, error) { return p.output.Read(b) }
func (p *processInput) Close() error {
	p.once.Do(func() { p.cancel(); p.server.Close(); p.output.Close(); <-p.done })
	return nil
}

var playlistURI = regexp.MustCompile(`URI="([^"]+)"`)
var anyPlaylistURI = regexp.MustCompile(`(?i)\bURI\s*=`)

func privatePlaylist(data []byte, id, base string) ([]byte, error) {
	if len(data) > 1024*1024 || !bytes.HasPrefix(data, []byte("#EXTM3U")) {
		return nil, ErrIntegrity
	}
	prefix := "/api/kore-edge/media/" + id + "/hls/"
	count := 0
	bad := false
	replace := func(ref string) string {
		count++
		if count > 1024 || !strings.HasPrefix(ref, prefix) {
			bad = true
			return ""
		}
		tail := strings.TrimPrefix(ref, prefix)
		if _, e := mediaPath(id, "hls", tail); e != nil {
			bad = true
			return ""
		}
		return base + tail
	}
	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "#") {
			if len(anyPlaylistURI.FindAllString(line, -1)) != len(playlistURI.FindAllString(line, -1)) {
				return nil, ErrIntegrity
			}
			line = playlistURI.ReplaceAllStringFunc(line, func(match string) string { return `URI="` + replace(playlistURI.FindStringSubmatch(match)[1]) + `"` })
		} else if strings.TrimSpace(line) != "" {
			line = replace(strings.TrimSpace(line))
		}
		lines[i] = line
	}
	if bad {
		return nil, ErrIntegrity
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// OpenHLS remuxes an authorized HLS origin to TS. FFmpeg only sees a private
// loopback relay and a playlist on stdin: neither origin secrets nor the local
// capability are placed in process arguments, DTOs or logs.
func (s *Service) OpenHLS(parent context.Context, id, ffmpeg string) (io.ReadCloser, error) {
	release, ok := mediaresources.Input()
	if !ok {
		return nil, mediaresources.ErrBusy
	}
	started := false
	defer func() {
		if !started {
			release()
		}
	}()

	if !s.allowed(id, "hls") {
		return nil, ErrDenied
	}
	ctx, cancel := context.WithCancel(parent)
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		cancel()
		return nil, ErrUnavailable
	}
	secret := make([]byte, 32)
	if _, e = rand.Read(secret); e != nil {
		listener.Close()
		cancel()
		return nil, ErrUnavailable
	}
	prefix := "/" + hex.EncodeToString(secret) + "/"
	base := "http://" + listener.Addr().String() + prefix
	var server *http.Server
	serve := func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, prefix) || r.URL.RawQuery != "" || !s.allowed(id, "hls") {
			w.WriteHeader(403)
			return
		}
		tail := strings.TrimPrefix(r.URL.Path, prefix)
		path, e := mediaPath(id, "hls", tail)
		if e != nil {
			w.WriteHeader(400)
			return
		}
		response, _, e := s.remote(r.Context(), path, r.Header.Get("Range"))
		if e != nil {
			w.WriteHeader(502)
			return
		}
		defer response.Body.Close()
		mime := response.Header.Get("Content-Type")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", mime)
		if strings.Contains(strings.ToLower(mime), "mpegurl") {
			body, e := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
			if e != nil {
				w.WriteHeader(502)
				return
			}
			body, e = privatePlaylist(body, id, base)
			if e != nil {
				w.WriteHeader(502)
				return
			}
			w.Write(body)
			return
		}
		if value := response.Header.Get("Content-Range"); value != "" {
			w.Header().Set("Content-Range", value)
		}
		w.WriteHeader(response.StatusCode)
		io.Copy(w, io.LimitReader(response.Body, 16*1024*1024))
	}
	server = &http.Server{Handler: http.HandlerFunc(serve), ReadHeaderTimeout: 2 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return ctx }}
	go func() { _ = server.Serve(listener) }()
	failed := func() { cancel(); server.Close() }
	first, _, e := s.remote(ctx, "/api/kore-edge/media/"+id+"/hls/index.m3u8")
	if e != nil {
		failed()
		return nil, e
	}
	playlist, e := io.ReadAll(io.LimitReader(first.Body, 1024*1024+1))
	first.Body.Close()
	if e != nil {
		failed()
		return nil, ErrUnavailable
	}
	playlist, e = privatePlaylist(playlist, id, base)
	if e != nil {
		failed()
		return nil, e
	}
	cmd := exec.CommandContext(ctx, ffmpeg, "-hide_banner", "-loglevel", "quiet", "-nostdin", "-protocol_whitelist", "pipe,http,tcp,crypto", "-format_whitelist", "hls,mpegts,aac,mov", "-allowed_extensions", "ALL", "-analyzeduration", "1500000", "-probesize", "2000000", "-re", "-f", "hls", "-i", "pipe:0", "-map", "0:v:0?", "-map", "0:a:0?", "-c", "copy", "-mpegts_flags", "+resend_headers", "-flush_packets", "1", "-f", "mpegts", "pipe:1")
	mediaresources.Configure(cmd)
	cmd.Stdin = strings.NewReader("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n" + base + "index.m3u8\n")
	cmd.WaitDelay = time.Second
	output, e := cmd.StdoutPipe()
	if e != nil {
		failed()
		return nil, ErrUnavailable
	}
	if cmd.Start() != nil {
		output.Close()
		failed()
		return nil, ErrUnavailable
	}
	process := &processInput{output: output, cancel: cancel, server: server, done: make(chan struct{})}
	started = true
	go func() { _ = cmd.Wait(); release(); close(process.done) }()
	return process, nil
}
