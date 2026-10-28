// Package output supplies optional protocol outputs from the existing shared ingest.
package output

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"kore-edge/internal/catalog"
	"kore-edge/internal/security"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrInvalid = errors.New("invalid_output_configuration")
var ErrConflict = errors.New("output_configuration_changed")
var ErrUnavailable = errors.New("output_unavailable")
var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type Source interface {
	SharedStream(context.Context, string) (io.ReadCloser, error)
	PlaybackAllowed(string, bool) bool
	IsActive(string) bool
}
type Keys interface{ Permanent(string) (string, error) }
type Network struct {
	Enabled    bool   `json:"enabled"`
	BindIP     string `json:"bind_ip"`
	PublicHost string `json:"public_host"`
	RTSP       int    `json:"rtsp_port"`
	RTMP       int    `json:"rtmp_port"`
	SRT        int    `json:"srt_port"`
}
type Channel struct {
	DASH       bool `json:"dash"`
	RTSP       bool `json:"rtsp"`
	RTMP       bool `json:"rtmp"`
	SRT        bool `json:"srt"`
	Segments   int  `json:"segments"`
	Duration   int  `json:"duration"`
	MaxReaders int  `json:"max_readers"`
}

func DefaultChannel() Channel { return Channel{Segments: 4, Duration: 2, MaxReaders: 16} }

type state struct {
	Version  int                `json:"version"`
	Revision uint64             `json:"revision"`
	Network  Network            `json:"network"`
	Channels map[string]Channel `json:"channels"`
}
type Status struct {
	Bridge     string    `json:"bridge"`
	Feed       string    `json:"feed"`
	DASH       string    `json:"dash"`
	LastPacket time.Time `json:"last_packet"`
}
type View struct {
	Revision uint64  `json:"revision"`
	Network  Network `json:"network"`
	Channel  Channel `json:"channel"`
	Status   Status  `json:"status"`
	MediaMTX bool    `json:"mediamtx_available"`
	FFmpeg   bool    `json:"ffmpeg_available"`
}
type Service struct {
	mu                                       sync.Mutex
	directory, runtime, mtx, ffmpeg, ffprobe string
	data                                     state
	source                                   Source
	keys                                     Keys
	ctx                                      context.Context
	cancel                                   context.CancelFunc
	done                                     chan struct{}
	wake                                     chan struct{}
	requests                                 chan struct{}
	jobs                                     map[string]*worker
	bridge                                   *bridge
	status                                   string
	bridgeAttempts                           int
	retryAt                                  time.Time
	auth                                     *authServer
}

func trusted(tool, name string) bool {
	if !filepath.IsAbs(tool) || strings.TrimSuffix(filepath.Base(tool), ".exe") != name {
		return false
	}
	p, e := filepath.EvalSymlinks(tool)
	if e != nil {
		return false
	}
	i, e := os.Stat(p)
	return e == nil && i.Mode().IsRegular() && (os.PathSeparator == '\\' || i.Mode().Perm()&0022 == 0)
}
func tools(name string) string {
	p, _ := exec.LookPath(name)
	if !trusted(p, name) {
		return ""
	}
	return p
}
func New(parent context.Context, directory string, source Source, keys Keys) (*Service, error) {
	if source == nil || keys == nil || security.PrivateDirectory(directory, true) != nil {
		return nil, ErrUnavailable
	}
	ctx, cancel := context.WithCancel(parent)
	s := &Service{directory: directory, source: source, keys: keys, ctx: ctx, cancel: cancel, done: make(chan struct{}), wake: make(chan struct{}, 1), requests: make(chan struct{}, 4), jobs: map[string]*worker{}, status: "disabled", data: state{Version: 1, Channels: map[string]Channel{}}, mtx: tools("mediamtx"), ffmpeg: tools("ffmpeg"), ffprobe: tools("ffprobe")}
	path := filepath.Join(directory, "outputs.json")
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		b, e := security.ReadPrivateFile(path, 512*1024)
		if e != nil || catalog.DecodeStrict(b, &s.data) != nil || s.data.Version != 1 || s.data.Channels == nil || len(s.data.Channels) > 512 || validateNetwork(s.data.Network) != nil {
			cancel()
			return nil, ErrUnavailable
		}
		for id, c := range s.data.Channels {
			if !safeID.MatchString(id) || !validChannel(c) {
				cancel()
				return nil, ErrUnavailable
			}
		}
	}
	root, e := os.MkdirTemp("", "ke-out-")
	if e != nil {
		cancel()
		return nil, ErrUnavailable
	}
	s.runtime = root
	// Even the private authentication socket is created only when a listener is enabled.
	go s.run()
	return s, nil
}
func validateNetwork(n Network) error {
	if !n.Enabled {
		return nil
	}
	ip, e := netip.ParseAddr(n.BindIP)
	if e != nil || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.Zone() != "" {
		return ErrInvalid
	}
	if addr, e := netip.ParseAddr(n.PublicHost); e == nil {
		if addr.Zone() != "" || addr.IsUnspecified() || addr.IsMulticast() {
			return ErrInvalid
		}
	} else {
		if len(n.PublicHost) > 253 || !regexp.MustCompile(`^[A-Za-z0-9.-]+$`).MatchString(n.PublicHost) {
			return ErrInvalid
		}
		for _, label := range strings.Split(n.PublicHost, ".") {
			if len(label) == 0 || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
				return ErrInvalid
			}
		}
	}
	ports := []int{n.RTSP, n.RTMP, n.SRT}
	count := 0
	for _, p := range ports {
		if p != 0 && (p < 1024 || p > 65535) {
			return ErrInvalid
		}
		if p > 0 {
			count++
		}
	}
	if count == 0 || n.RTSP > 0 && n.RTSP == n.RTMP {
		return ErrInvalid
	}
	return nil
}
func validChannel(c Channel) bool {
	return c.Segments >= 2 && c.Segments <= 10 && c.Duration >= 1 && c.Duration <= 6 && c.MaxReaders >= 1 && c.MaxReaders <= 32
}
func (s *Service) Snapshot(id string) View {
	s.mu.Lock()
	defer s.mu.Unlock()
	c, ok := s.data.Channels[id]
	if !ok {
		c = DefaultChannel()
	}
	v := View{Revision: s.data.Revision, Network: s.data.Network, Channel: c, Status: Status{Bridge: s.status, Feed: "disabled", DASH: "disabled"}, MediaMTX: s.mtx != "", FFmpeg: s.ffmpeg != "" && s.ffprobe != ""}
	if c.DASH {
		v.Status.DASH = "queued"
	}
	if s.data.Network.Enabled && (c.RTSP || c.RTMP || c.SRT) {
		v.Status.Feed = "queued"
	}
	if !s.source.IsActive(id) {
		if c.DASH {
			v.Status.DASH = "stopped"
		}
		if c.RTSP || c.RTMP || c.SRT {
			v.Status.Feed = "stopped"
		}
	}
	if j := s.jobs[id]; j != nil {
		j.mu.Lock()
		v.Status.Feed = j.feedState
		v.Status.DASH = j.dashState
		v.Status.LastPacket = j.last
		j.mu.Unlock()
	}
	if v.Status.Feed == "sending" && time.Since(v.Status.LastPacket) > 3*time.Second {
		v.Status.Feed = "waiting_signal"
	}
	return v
}
func (s *Service) Save(id string, c Channel, n *Network, expected uint64) error {
	if !safeID.MatchString(id) || !validChannel(c) || !s.source.PlaybackAllowed(id, false) {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx.Err() != nil {
		return ErrUnavailable
	}
	if expected != s.data.Revision {
		return ErrConflict
	}
	next := s.data
	next.Channels = map[string]Channel{}
	for k, v := range s.data.Channels {
		next.Channels[k] = v
	}
	if n != nil {
		if validateNetwork(*n) != nil {
			return ErrInvalid
		}
		next.Network = *n
	}
	if next.Network.Enabled && (s.mtx == "" || os.PathSeparator == '\\') {
		return ErrUnavailable
	}
	if c.DASH && (s.ffmpeg == "" || s.ffprobe == "" || !s.source.PlaybackAllowed(id, true)) {
		return ErrUnavailable
	}
	if next.Network.Enabled && ((c.RTSP && next.Network.RTSP == 0) || (c.RTMP && next.Network.RTMP == 0) || (c.SRT && next.Network.SRT == 0)) {
		return ErrInvalid
	}
	next.Channels[id] = c
	if len(next.Channels) > 512 {
		return ErrInvalid
	}
	if reflect.DeepEqual(next, s.data) {
		return nil
	}
	next.Revision++
	b, e := json.Marshal(next)
	if e != nil {
		return ErrUnavailable
	}
	if atomicWrite(s.directory, "outputs.json", b) != nil {
		return ErrUnavailable
	}
	s.data = next
	select {
	case s.wake <- struct{}{}:
	default:
	}
	return nil
}
func atomicWrite(dir, name string, b []byte) error {
	f, e := os.CreateTemp(dir, ".output-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return e
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	return os.Rename(f.Name(), filepath.Join(dir, name))
}
func (s *Service) Links(id string) map[string]string {
	v := s.Snapshot(id)
	out := map[string]string{}
	key, e := s.keys.Permanent(id)
	if e != nil {
		return out
	}
	host := v.Network.PublicHost
	if v.Channel.RTSP && v.Network.Enabled {
		out["rtsp"] = "rtsp://edge:" + key + "@" + net.JoinHostPort(host, strconv.Itoa(v.Network.RTSP)) + "/" + id
	}
	if v.Channel.RTMP && v.Network.Enabled {
		out["rtmp"] = "rtmp://" + net.JoinHostPort(host, strconv.Itoa(v.Network.RTMP)) + "/" + id + "?user=edge&pass=" + key
	}
	if v.Channel.SRT && v.Network.Enabled {
		out["srt"] = "srt://" + net.JoinHostPort(host, strconv.Itoa(v.Network.SRT)) + "?streamid=read:" + id + ":edge:" + key + "&passphrase=" + key + "&pbkeylen=32&mode=caller&pkt_size=1316"
	}
	return out
}
func (s *Service) Close() { s.cancel(); <-s.done }
