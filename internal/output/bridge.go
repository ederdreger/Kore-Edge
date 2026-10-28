package output

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

type authServer struct {
	server  *http.Server
	address string
}
type bridge struct {
	cancel   context.CancelFunc
	done     chan struct{}
	network  Network
	document string
	started  time.Time
}
type worker struct {
	mu                   sync.Mutex
	ctx                  context.Context
	cancel               context.CancelFunc
	done                 chan struct{}
	config               Channel
	socket, dir          string
	feedState, dashState string
	last                 time.Time
}

func socketName(root, id string) string {
	h := sha256.Sum256([]byte(id))
	return filepath.Join(root, hex.EncodeToString(h[:8])+".sock")
}
func (s *Service) startAuth() error {
	if s.auth != nil {
		return nil
	}
	ln, e := net.Listen("tcp4", "127.0.0.1:0")
	if e != nil {
		return e
	}
	server := &http.Server{Handler: http.HandlerFunc(s.authenticate), ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 3 * time.Second, WriteTimeout: 3 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 8192, ErrorLog: log.New(io.Discard, "", 0)}
	s.auth = &authServer{server: server, address: "http://" + ln.Addr().String() + "/auth"}
	go func() { _ = server.Serve(ln) }()
	return nil
}
func (s *Service) authenticate(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var in struct {
		User     string `json:"user"`
		Password string `json:"password"`
		Action   string `json:"action"`
		Path     string `json:"path"`
		Protocol string `json:"protocol"`
	}
	b, e := io.ReadAll(io.LimitReader(r.Body, 8193))
	if e != nil || len(b) > 8192 || r.Method != "POST" || r.URL.Path != "/auth" || json.Unmarshal(b, &in) != nil || in.Action != "read" || in.User != "edge" || !safeID.MatchString(in.Path) {
		w.WriteHeader(403)
		return
	}
	s.mu.Lock()
	c, ok := s.data.Channels[in.Path]
	enabled := s.data.Network.Enabled
	s.mu.Unlock()
	allowed := in.Protocol == "rtsp" && c.RTSP || in.Protocol == "rtmp" && c.RTMP || in.Protocol == "srt" && c.SRT
	if !ok || !enabled || !allowed || !s.source.IsActive(in.Path) || !s.source.PlaybackAllowed(in.Path, false) {
		w.WriteHeader(403)
		return
	}
	key, e := s.keys.Permanent(in.Path)
	if e != nil || subtle.ConstantTimeCompare([]byte(key), []byte(in.Password)) != 1 {
		w.WriteHeader(401)
		return
	}
	w.WriteHeader(204)
}
func (s *Service) bridgeDocument(paths map[string]Channel) ([]byte, error) {
	n := s.data.Network
	config := map[string]any{"logLevel": "error", "logDestinations": []string{"stdout"}, "readTimeout": "8s", "writeTimeout": "8s", "writeQueueSize": 256, "authMethod": "http", "authHTTPAddress": s.auth.address, "authHTTPExclude": []any{}, "api": false, "metrics": false, "pprof": false, "playback": false, "hls": false, "webrtc": false, "moq": false, "rtsp": n.RTSP > 0, "rtmp": n.RTMP > 0, "srt": n.SRT > 0, "rtspTransports": []string{"tcp"}, "paths": map[string]any{}}
	if n.RTSP > 0 {
		config["rtspAddress"] = net.JoinHostPort(n.BindIP, strconv.Itoa(n.RTSP))
	}
	if n.RTMP > 0 {
		config["rtmpAddress"] = net.JoinHostPort(n.BindIP, strconv.Itoa(n.RTMP))
	}
	if n.SRT > 0 {
		config["srtAddress"] = net.JoinHostPort(n.BindIP, strconv.Itoa(n.SRT))
	}
	items := config["paths"].(map[string]any)
	for id, c := range paths {
		key, e := s.keys.Permanent(id)
		if e != nil {
			return nil, e
		}
		items[id] = map[string]any{"source": "unix+mpegts://" + filepath.ToSlash(socketName(s.runtime, id)), "sourceOnDemand": false, "maxReaders": c.MaxReaders, "srtReadPassphrase": key}
	}
	return json.Marshal(config)
}
func (s *Service) stopBridge() {
	if b := s.bridge; b != nil {
		b.cancel()
		<-b.done
		s.bridge = nil
	}
}
func (s *Service) ensureBridge(paths map[string]Channel) {
	if !s.data.Network.Enabled || len(paths) == 0 {
		s.stopBridge()
		s.status = "disabled"
		return
	}
	if s.mtx == "" || os.PathSeparator == '\\' {
		s.status = "tool_unavailable"
		return
	}
	if s.startAuth() != nil {
		s.status = "failed"
		return
	}
	document, e := s.bridgeDocument(paths)
	if e != nil {
		s.status = "failed"
		return
	}
	if b := s.bridge; b != nil {
		select {
		case <-b.done:
			s.bridge = nil
			s.bridgeAttempts++
			delay := time.Duration(1<<min(s.bridgeAttempts, 5)) * time.Second
			if delay > 30*time.Second {
				delay = 30 * time.Second
			}
			s.retryAt = time.Now().Add(delay)
			s.status = "reconnecting"
			return
		default:
		}
		if !reflect.DeepEqual(b.network, s.data.Network) {
			s.stopBridge()
		} else {
			if b.document != string(document) {
				if atomicWrite(s.runtime, "mediamtx.json", document) != nil {
					s.status = "failed"
					return
				}
				b.document = string(document)
			}
			s.status = "listening"
			if time.Since(b.started) > 30*time.Second {
				s.bridgeAttempts = 0
			}
			return
		}
	}
	if time.Now().Before(s.retryAt) {
		s.status = "reconnecting"
		return
	}
	if atomicWrite(s.runtime, "mediamtx.json", document) != nil {
		s.status = "failed"
		return
	}
	stale, _ := filepath.Glob(filepath.Join(s.runtime, "*.sock"))
	for _, path := range stale {
		if info, e := os.Lstat(path); e == nil && info.Mode()&os.ModeSocket != 0 {
			os.Remove(path)
		}
	}
	ctx, cancel := context.WithCancel(s.ctx)
	cmd := exec.CommandContext(ctx, s.mtx, filepath.Join(s.runtime, "mediamtx.json"))
	cmd.Dir = s.runtime
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "MTX_") && !strings.HasPrefix(v, "GOMAXPROCS=") && !strings.HasPrefix(v, "GOMEMLIMIT=") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "GOMAXPROCS=2", "GOMEMLIMIT=256MiB")
	if cmd.Start() != nil {
		cancel()
		s.status = "failed"
		return
	}
	b := &bridge{cancel: cancel, done: make(chan struct{}), network: s.data.Network, document: string(document), started: time.Now()}
	s.bridge = b
	s.status = "starting"
	go func() { _ = cmd.Wait(); cancel(); close(b.done) }()
}
func (s *Service) run() {
	defer close(s.done)
	defer os.RemoveAll(s.runtime)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.ctx.Done():
			s.mu.Lock()
			for _, j := range s.jobs {
				j.cancel()
			}
			s.stopBridge()
			for _, j := range s.jobs {
				<-j.done
			}
			if s.auth != nil {
				s.auth.server.Close()
			}
			s.mu.Unlock()
			return
		case <-ticker.C:
		case <-s.wake:
		}
		s.mu.Lock()
		desired := map[string]Channel{}
		paths := map[string]Channel{}
		for id, c := range s.data.Channels {
			if s.source.IsActive(id) && s.source.PlaybackAllowed(id, false) && (c.DASH || s.data.Network.Enabled && (c.RTSP || c.RTMP || c.SRT)) {
				desired[id] = c
				if !s.data.Network.Enabled {
					copy := c
					copy.RTSP = false
					copy.RTMP = false
					copy.SRT = false
					desired[id] = copy
				}
				if s.data.Network.Enabled && (c.RTSP || c.RTMP || c.SRT) {
					paths[id] = c
				}
			}
		}
		for id, j := range s.jobs {
			c, ok := desired[id]
			if !ok || !reflect.DeepEqual(c, j.config) {
				j.cancel()
				<-j.done
				delete(s.jobs, id)
			}
		}
		s.ensureBridge(paths)
		for id, c := range desired {
			if s.jobs[id] != nil || len(s.jobs) >= 32 {
				continue
			}
			ctx, cancel := context.WithCancel(s.ctx)
			j := &worker{ctx: ctx, cancel: cancel, done: make(chan struct{}), config: c, socket: socketName(s.runtime, id), feedState: "disabled", dashState: "disabled"}
			s.jobs[id] = j
			go s.work(id, j)
		}
		s.mu.Unlock()
	}
}
func (s *Service) work(id string, j *worker) {
	var wg sync.WaitGroup
	if j.config.RTSP || j.config.RTMP || j.config.SRT {
		wg.Add(1)
		go func() { defer wg.Done(); s.feed(id, j) }()
	}
	if j.config.DASH {
		wg.Add(1)
		go func() { defer wg.Done(); s.packageStream(id, j) }()
	}
	wg.Wait()
	close(j.done)
}
func (s *Service) feed(id string, j *worker) {
	for {
		if j.ctx.Err() != nil {
			return
		}
		j.mu.Lock()
		j.feedState = "connecting"
		j.mu.Unlock()
		conn, e := net.DialTimeout("unix", j.socket, time.Second)
		if e == nil {
			input, err := s.source.SharedStream(j.ctx, id)
			if err == nil {
				child, cancel := context.WithCancel(j.ctx)
				go func() { <-child.Done(); conn.Close(); input.Close() }()
				buf := make([]byte, 32*1024)
				for {
					n, err := input.Read(buf)
					if n > 0 {
						conn.SetWriteDeadline(time.Now().Add(2 * time.Second))
						if _, e = conn.Write(buf[:n]); e != nil {
							break
						}
						j.mu.Lock()
						j.feedState = "sending"
						j.last = time.Now().UTC()
						j.mu.Unlock()
					}
					if err != nil {
						break
					}
				}
				cancel()
				input.Close()
			}
			conn.Close()
		}
		j.mu.Lock()
		j.feedState = "reconnecting"
		j.mu.Unlock()
		select {
		case <-j.ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
