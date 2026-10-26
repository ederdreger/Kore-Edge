package hls

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var idPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)
var segmentPattern = regexp.MustCompile(`^seg[0-9]{9,20}\.ts$`)

func removeOwned(root, path string) error {
	rel, e := filepath.Rel(root, path)
	if e != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrUnavailable
	}
	return os.RemoveAll(path)
}

const MaxSegmentBytes = 8 * 1024 * 1024

type Authorizer func(string, time.Time) bool

// Ready reports an actual playlist, rather than just a started subprocess.
func (m *Manager) Ready(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.sessions[id]
	if s == nil || m.closed {
		return false
	}
	info, e := os.Lstat(filepath.Join(s.dir, "index.m3u8"))
	return e == nil && info.Mode().IsRegular() && info.Size() > 0 && time.Since(info.ModTime()) < 15*time.Second
}

type Manager struct {
	mu       sync.Mutex
	config   Config
	root     string
	allow    Authorizer
	sessions map[string]*Session
	closed   bool
	requests chan struct{}
}
type Session struct {
	mu        sync.Mutex
	closeOnce sync.Once
	closeErr  error
	owner     *Manager
	id, dir   string
	ctx       context.Context
	cancel    context.CancelFunc
	queue     chan [188]byte
	done      chan struct{}
	failed    bool
	closed    bool
}

func New(ctx context.Context, c Config, allow Authorizer) (*Manager, error) {
	if c.Validate() != nil || !c.Enabled || allow == nil {
		return nil, ErrUnavailable
	}
	if Verify(ctx, c) != nil {
		return nil, ErrUnavailable
	}
	return newStorage(c, allow)
}
func newStorage(c Config, allow Authorizer) (*Manager, error) {
	if e := os.MkdirAll(c.Root, 0700); e != nil {
		return nil, ErrUnavailable
	}
	info, e := os.Lstat(c.Root)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, ErrUnavailable
	}
	root, e := os.MkdirTemp(c.Root, "kore-hls-")
	if e != nil {
		return nil, ErrUnavailable
	}
	return &Manager{config: c, root: root, allow: allow, sessions: map[string]*Session{}, requests: make(chan struct{}, 8)}, nil
}
func (m *Manager) Open(ctx context.Context, id string) (*Session, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed || !idPattern.MatchString(id) || !m.allow(id, time.Now().UTC()) || m.sessions[id] != nil || len(m.sessions) >= 4 {
		return nil, ErrUnavailable
	}
	dir, e := os.MkdirTemp(m.root, id+"-")
	if e != nil {
		return nil, ErrUnavailable
	}
	child, cancel := context.WithCancel(ctx)
	s := &Session{owner: m, id: id, dir: dir, ctx: child, cancel: cancel, queue: make(chan [188]byte, 128), done: make(chan struct{})}
	cmd := exec.CommandContext(child, m.config.FFmpeg, segmentArgs()...)
	cmd.Dir = dir
	cmd.WaitDelay = time.Second
	input, e := cmd.StdinPipe()
	if e != nil {
		cancel()
		removeOwned(m.root, dir)
		return nil, ErrUnavailable
	}
	if cmd.Start() != nil {
		input.Close()
		cancel()
		removeOwned(m.root, dir)
		return nil, ErrUnavailable
	}
	m.sessions[id] = s
	go func() {
		defer input.Close()
		w := bufio.NewWriterSize(input, 1316)
		for {
			select {
			case <-child.Done():
				return
			case packet := <-s.queue:
				if _, e := w.Write(packet[:]); e != nil {
					cancel()
					return
				}
				if w.Buffered() >= 1316 {
					if w.Flush() != nil {
						cancel()
						return
					}
				}
			}
		}
	}()
	go func() {
		_ = cmd.Wait()
		cancel()
		s.mu.Lock()
		s.failed = true
		s.mu.Unlock()
		close(s.done)
	}()
	go s.monitor()
	return s, nil
}
func (s *Session) monitor() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	last := time.Now()
	var mod time.Time
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if !s.owner.allow(s.id, time.Now().UTC()) {
				s.cancel()
				return
			}
			entries, e := os.ReadDir(s.dir)
			if e != nil || len(entries) > 16 {
				s.cancel()
				return
			}
			var size int64
			for _, entry := range entries {
				info, e := entry.Info()
				if e != nil || !info.Mode().IsRegular() || info.Size() > MaxSegmentBytes {
					s.cancel()
					return
				}
				size += info.Size()
				if entry.Name() == "index.m3u8" && info.ModTime().After(mod) {
					mod = info.ModTime()
					last = time.Now()
				}
			}
			if size > 32*1024*1024 || time.Since(last) > 10*time.Second {
				s.cancel()
				return
			}
		}
	}
}
func (s *Session) WritePacket(packet []byte) error {
	if len(packet) != 188 || packet[0] != 0x47 || !s.owner.allow(s.id, time.Now().UTC()) {
		return ErrUnavailable
	}
	s.mu.Lock()
	bad := s.failed || s.closed
	s.mu.Unlock()
	if bad {
		return ErrUnavailable
	}
	var copyPacket [188]byte
	copy(copyPacket[:], packet)
	timer := time.NewTimer(100 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-s.ctx.Done():
		return ErrUnavailable
	case <-timer.C:
		s.cancel()
		return ErrUnavailable
	case s.queue <- copyPacket:
		return nil
	}
}
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		s.closed = true
		s.mu.Unlock()
		s.cancel()
		<-s.done
		m := s.owner
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.sessions[s.id] == s {
			delete(m.sessions, s.id)
		}
		s.closeErr = removeOwned(m.root, s.dir)
	})
	return s.closeErr
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	sessions := []*Session{}
	for _, s := range m.sessions {
		sessions = append(sessions, s)
	}
	m.mu.Unlock()
	for _, s := range sessions {
		_ = s.Close()
	}
	_ = removeOwned(m.config.Root, m.root)
}
