package manualchannels

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var uriAttribute = regexp.MustCompile(`URI="([^"]+)"`)
var anyURI = regexp.MustCompile(`(?i)\bURI\s*=`)
var suffix = regexp.MustCompile(`\.(m3u8|ts|m4s|mp4|aac|vtt)$`)

func random() string {
	b := make([]byte, 24)
	if _, e := rand.Read(b); e != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

type references struct {
	mu    sync.Mutex
	items map[string]string
	order []string
	root  *url.URL
	base  string
}

func (r *references) add(raw string, from *url.URL) (string, error) {
	if len(raw) > 4096 || strings.ContainsAny(raw, "\r\n\x00") {
		return "", ErrInvalid
	}
	ref, e := url.Parse(raw)
	if e != nil {
		return "", ErrInvalid
	}
	ref = from.ResolveReference(ref)
	if ref.Scheme != r.root.Scheme || ref.Host != r.root.Host || ref.Fragment != "" {
		return "", ErrDenied
	}
	// Same-origin Basic credentials are inherited only within this private relay.
	if ref.User == nil {
		ref.User = r.root.User
	}
	key := random()
	if key == "" {
		return "", ErrUnavailable
	}
	extension := suffix.FindString(strings.ToLower(ref.Path))
	if extension == "" {
		extension = ".ts"
	}
	key += extension
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.order) >= 1024 {
		delete(r.items, r.order[0])
		r.order = r.order[1:]
	}
	r.items[key] = ref.String()
	r.order = append(r.order, key)
	return r.base + key, nil
}
func (r *references) get(key string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	target := r.items[key]
	if target != "" {
		for i, item := range r.order {
			if item == key {
				r.order = append(append(r.order[:i:i], r.order[i+1:]...), key)
				break
			}
		}
	}
	return target
}
func (r *references) rewrite(data []byte, from *url.URL) ([]byte, error) {
	if len(data) > 1024*1024 || !bytes.HasPrefix(data, []byte("#EXTM3U")) {
		return nil, ErrInvalid
	}
	lines := strings.Split(string(data), "\n")
	count := 0
	for i, line := range lines {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(line, "#") {
			if len(anyURI.FindAllString(line, -1)) != len(uriAttribute.FindAllString(line, -1)) {
				return nil, ErrInvalid
			}
			var failed error
			line = uriAttribute.ReplaceAllStringFunc(line, func(match string) string {
				count++
				next, e := r.add(uriAttribute.FindStringSubmatch(match)[1], from)
				if e != nil {
					failed = e
				}
				return `URI="` + next + `"`
			})
			if failed != nil {
				return nil, failed
			}
		} else if strings.TrimSpace(line) != "" {
			count++
			next, e := r.add(strings.TrimSpace(line), from)
			if e != nil {
				return nil, e
			}
			line = next
		}
		if count > 512 {
			return nil, ErrInvalid
		}
		lines[i] = line
	}
	return []byte(strings.Join(lines, "\n")), nil
}

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
func executable(raw string) bool {
	if !filepath.IsAbs(raw) || (filepath.Base(raw) != "ffmpeg" && filepath.Base(raw) != "ffmpeg.exe") {
		return false
	}
	p, e := filepath.EvalSymlinks(raw)
	if e != nil {
		return false
	}
	info, e := os.Stat(p)
	return e == nil && info.Mode().IsRegular() && (os.PathSeparator == '\\' || info.Mode().Perm()&0022 == 0)
}
func (s *Service) openHLS(parent context.Context, c Source, ffmpeg string) (io.ReadCloser, error) {
	if !executable(ffmpeg) {
		return nil, ErrUnavailable
	}
	root, e := parse(c.URL)
	if e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(parent)
	listener, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		cancel()
		return nil, ErrUnavailable
	}
	capability := random()
	if capability == "" {
		cancel()
		listener.Close()
		return nil, ErrUnavailable
	}
	prefix := "/" + capability + "/"
	refs := &references{items: map[string]string{}, root: root, base: "http://" + listener.Addr().String() + prefix}
	slots := make(chan struct{}, 8)
	server := &http.Server{ReadHeaderTimeout: 2 * time.Second, WriteTimeout: 8 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192, BaseContext: func(net.Listener) context.Context { return ctx }}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" || !strings.HasPrefix(r.URL.Path, prefix) || r.URL.RawQuery != "" {
			w.WriteHeader(403)
			return
		}
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			w.WriteHeader(429)
			return
		}
		key := strings.TrimPrefix(r.URL.Path, prefix)
		target := c.URL
		if key != "index.m3u8" {
			target = refs.get(key)
			if target == "" {
				w.WriteHeader(404)
				return
			}
		}
		rangeValue := r.Header.Get("Range")
		if rangeValue != "" && !regexp.MustCompile(`^bytes=\d+-\d*$`).MatchString(rangeValue) {
			w.WriteHeader(400)
			return
		}
		response, e := s.openHTTP(r.Context(), target, rangeValue)
		if e != nil {
			w.WriteHeader(502)
			return
		}
		defer response.Body.Close()
		mime := response.Header.Get("Content-Type")
		u, _ := parse(target)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", mime)
		if strings.Contains(strings.ToLower(mime), "mpegurl") || strings.HasSuffix(strings.ToLower(u.Path), ".m3u8") {
			data, e := io.ReadAll(io.LimitReader(response.Body, 1024*1024+1))
			if e != nil {
				w.WriteHeader(502)
				return
			}
			data, e = refs.rewrite(data, u)
			if e != nil {
				w.WriteHeader(502)
				return
			}
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write(data)
			return
		}
		if value := response.Header.Get("Content-Range"); value != "" {
			w.Header().Set("Content-Range", value)
		}
		w.WriteHeader(response.StatusCode)
		_, _ = io.Copy(w, io.LimitReader(response.Body, 32*1024*1024))
	})
	go func() { _ = server.Serve(listener) }()
	// A master playlist must reference media playlists, not another master.
	// Media-only input instead needs a small master so FFmpeg can reload its URL.
	first, e := s.openHTTP(ctx, c.URL, "")
	if e != nil {
		cancel()
		server.Close()
		return nil, ErrUnavailable
	}
	document, e := io.ReadAll(io.LimitReader(first.Body, 1024*1024+1))
	first.Body.Close()
	if e != nil || len(document) > 1024*1024 || !bytes.HasPrefix(document, []byte("#EXTM3U")) {
		cancel()
		server.Close()
		return nil, ErrInvalid
	}
	initial := "#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n" + refs.base + "index.m3u8\n"
	if bytes.Contains(document, []byte("#EXT-X-STREAM-INF:")) {
		rewritten, err := refs.rewrite(document, root)
		if err != nil {
			cancel()
			server.Close()
			return nil, err
		}
		initial = string(rewritten)
	}
	cmd := s.command(ctx, ffmpeg, "-hide_banner", "-loglevel", "quiet", "-nostdin", "-protocol_whitelist", "pipe,http,tcp,crypto", "-format_whitelist", "hls,mpegts,aac,mov", "-allowed_extensions", "ALL", "-analyzeduration", "1000000", "-probesize", "1000000", "-re", "-f", "hls", "-i", "pipe:0", "-map", "0:v:0?", "-map", "0:a:0?", "-c", "copy", "-mpegts_flags", "+resend_headers", "-flush_packets", "1", "-f", "mpegts", "pipe:1")
	cmd.Stdin = strings.NewReader(initial)
	if cmd.Stderr == nil {
		cmd.Stderr = io.Discard
	}
	cmd.WaitDelay = time.Second
	output, e := cmd.StdoutPipe()
	if e != nil {
		cancel()
		server.Close()
		return nil, ErrUnavailable
	}
	if cmd.Start() != nil {
		cancel()
		server.Close()
		output.Close()
		return nil, ErrUnavailable
	}
	p := &processInput{output: output, cancel: cancel, server: server, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); close(p.done); cancel(); server.Close() }()
	return p, nil
}
