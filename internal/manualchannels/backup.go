package manualchannels

import (
	"bytes"
	"context"
	"io"
	"kore-edge/internal/mediaresources"
	"kore-edge/internal/security"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Packet watchdogs observe PES activity, not decoded picture quality.
type packetWatch struct {
	io.ReadCloser
	last    atomic.Int64
	video   atomic.Int64
	audio   atomic.Int64
	pending []byte
	stop    chan struct{}
	once    sync.Once
}

func watchPackets(ctx context.Context, r io.ReadCloser, source, video, audio int) io.ReadCloser {
	if source == 0 && video == 0 && audio == 0 {
		return r
	}
	w := &packetWatch{ReadCloser: r, stop: make(chan struct{})}
	now := time.Now().UnixNano()
	w.last.Store(now)
	w.video.Store(now)
	w.audio.Store(now)
	go func() {
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-w.stop:
				return
			case <-ctx.Done():
				w.Close()
				return
			case now := <-ticker.C:
				if source > 0 && now.Sub(time.Unix(0, w.last.Load())) > time.Duration(source)*time.Second || video > 0 && now.Sub(time.Unix(0, w.video.Load())) > time.Duration(video)*time.Second || audio > 0 && now.Sub(time.Unix(0, w.audio.Load())) > time.Duration(audio)*time.Second {
					w.Close()
					return
				}
			}
		}
	}()
	return w
}
func (w *packetWatch) Read(p []byte) (int, error) {
	n, e := w.ReadCloser.Read(p)
	if n > 0 {
		now := time.Now().UnixNano()
		w.last.Store(now)
		data := append(w.pending, p[:n]...)
		for len(data) >= 188 {
			if data[0] != 0x47 {
				data = data[1:]
				continue
			}
			packet := data[:188]
			data = data[188:]
			if packet[1]&0x40 == 0 || packet[3]&0x10 == 0 {
				continue
			}
			offset := 4
			if packet[3]&0x20 != 0 {
				offset += 1 + int(packet[4])
			}
			if offset+4 > 188 {
				continue
			}
			pes := packet[offset:]
			if !bytes.Equal(pes[:3], []byte{0, 0, 1}) {
				continue
			}
			if pes[3] >= 0xe0 && pes[3] <= 0xef {
				w.video.Store(now)
			}
			if pes[3] >= 0xc0 && pes[3] <= 0xdf || pes[3] == 0xbd {
				w.audio.Store(now)
			}
		}
		w.pending = append(w.pending[:0], data...)
	}
	return n, e
}
func (w *packetWatch) Close() error {
	var e error
	w.once.Do(func() { close(w.stop); e = w.ReadCloser.Close() })
	return e
}

func (s *Service) openBackup(ctx context.Context, id string, p InputPolicy, ffmpeg string) (io.ReadCloser, error) {
	path, e := s.assetPath(p.BackupAsset, "backup")
	if e != nil || !executable(ffmpeg) {
		return nil, ErrUnavailable
	}
	select {
	case s.slots <- struct{}{}:
	default:
		return nil, mediaresources.ErrBusy
	}
	child, cancel := context.WithCancel(ctx)
	cmd := s.command(child, ffmpeg, "-hide_banner", "-loglevel", "quiet", "-nostdin", "-protocol_whitelist", "file,pipe", "-stream_loop", "-1", "-re", "-f", "mov", "-i", path, "-map", "0:v:0", "-map", "0:a:0?", "-c", "copy", "-mpegts_flags", "+resend_headers+initial_discontinuity", "-f", "mpegts", "pipe:1")
	mediaresources.Configure(cmd)
	cmd.Stderr = io.Discard
	cmd.WaitDelay = time.Second
	out, e := cmd.StdoutPipe()
	if e != nil {
		cancel()
		<-s.slots
		return nil, ErrUnavailable
	}
	if cmd.Start() != nil {
		cancel()
		out.Close()
		<-s.slots
		return nil, ErrUnavailable
	}
	r := &backupProcess{ReadCloser: out, cancel: cancel, done: make(chan struct{})}
	go func() { _ = cmd.Wait(); <-s.slots; close(r.done) }()
	var result io.ReadCloser = contextual(ctx, r)
	if p.RecordBackup {
		result = s.recordBackup(id, result)
	}
	return result, nil
}

type backupProcess struct {
	io.ReadCloser
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func (r *backupProcess) Close() error {
	r.once.Do(func() { r.cancel(); r.ReadCloser.Close(); <-r.done })
	return nil
}

type recordingReader struct {
	io.ReadCloser
	file      *os.File
	remaining int64
	once      sync.Once
}

func (r *recordingReader) Read(b []byte) (int, error) {
	n, e := r.ReadCloser.Read(b)
	if n > 0 && r.file != nil && r.remaining > 0 {
		count := int64(n)
		if count > r.remaining {
			count = r.remaining
		}
		if _, e := r.file.Write(b[:count]); e != nil {
			r.remaining = 0
		} else {
			r.remaining -= count
		}
	}
	return n, e
}
func (r *recordingReader) Close() error {
	var e error
	r.once.Do(func() {
		e = r.ReadCloser.Close()
		if r.file != nil {
			r.file.Close()
		}
	})
	return e
}
func (s *Service) recordBackup(id string, r io.ReadCloser) io.ReadCloser {
	if !localID.MatchString(id) {
		return r
	}
	dir := filepath.Join(s.directory, "recordings")
	if security.PrivateDirectory(dir, true) != nil {
		return r
	}
	// At most 16 recordings, one bounded file per channel. Failures never stop media.
	s.assetsMu.Lock()
	defer s.assetsMu.Unlock()
	entries, e := os.ReadDir(dir)
	if e != nil {
		return r
	}
	path := filepath.Join(dir, id+".ts")
	if _, e = os.Lstat(path); os.IsNotExist(e) && len(entries) >= 16 {
		return r
	}
	f, e := os.CreateTemp(dir, ".record-")
	if e != nil {
		return r
	}
	f.Chmod(0600)
	if os.Rename(f.Name(), path) != nil {
		f.Close()
		os.Remove(f.Name())
		return r
	}
	return &recordingReader{ReadCloser: r, file: f, remaining: 32 << 20}
}

type prefixReader struct {
	io.Reader
	io.Closer
}

func (s *Service) withBackup(parent context.Context, id string, p Processing, initial io.ReadCloser, initialErr error, ffmpeg string) (io.ReadCloser, error) {
	if _, e := s.assetPath(p.Policy.BackupAsset, "backup"); e != nil {
		return nil, e
	}
	ctx, cancel := context.WithCancel(parent)
	reader, writer := io.Pipe()
	done := make(chan struct{})
	result := &switchReader{PipeReader: reader, cancel: cancel, done: done}
	go func() {
		defer close(done)
		defer writer.Close()
		defer cancel()
		current := initial
		if current != nil {
			current = contextual(ctx, current)
		}
		backup := initialErr != nil || initial == nil
		buffer := make([]byte, 188*7)
		for ctx.Err() == nil {
			var healthy chan io.ReadCloser
			var probeCancel context.CancelFunc
			if backup {
				var e error
				current, e = s.openBackup(ctx, id, p.Policy, ffmpeg)
				if e != nil {
					writer.CloseWithError(e)
					return
				}
				s.setBackup(id, true)
				healthy = make(chan io.ReadCloser, 1)
				var probeCtx context.Context
				probeCtx, probeCancel = context.WithCancel(ctx)
				go s.probeRecovery(probeCtx, ctx, id, p, ffmpeg, healthy)
			} else {
				s.setBackup(id, false)
			}
			failed := false
			for ctx.Err() == nil {
				if healthy != nil {
					select {
					case next := <-healthy:
						current.Close()
						current = contextual(ctx, next)
						backup = false
						probeCancel()
						healthy = nil
						s.setBackup(id, false)
					default:
					}
				}
				n, e := current.Read(buffer)
				if n > 0 {
					s.setBackup(id, backup)
					if _, err := writer.Write(buffer[:n]); err != nil {
						current.Close()
						if probeCancel != nil {
							probeCancel()
						}
						return
					}
				}
				if e != nil {
					failed = true
					break
				}
			}
			current.Close()
			if probeCancel != nil {
				probeCancel()
			}
			if healthy != nil {
				select {
				case extra := <-healthy:
					extra.Close()
				default:
				}
			}
			if ctx.Err() != nil {
				return
			}
			if !failed {
				return
			}
			backup = true
		}
	}()
	return contextual(parent, result), nil
}

type switchReader struct {
	*io.PipeReader
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func (r *switchReader) Close() error {
	r.once.Do(func() { r.cancel(); r.PipeReader.Close(); <-r.done })
	return nil
}
func (s *Service) setBackup(id string, active bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if m := s.mediaInfo[id]; m != nil {
		m.backup = active
	}
}
func (s *Service) probeRecovery(ctx, lifetime context.Context, id string, p Processing, ffmpeg string, out chan<- io.ReadCloser) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		for _, in := range p.Inputs {
			attempt, stop := context.WithCancel(lifetime)
			timer := time.AfterFunc(5*time.Second, stop)
			r, e := s.openInput(attempt, id, in, ffmpeg)
			if e != nil {
				timer.Stop()
				stop()
				continue
			}
			r = watchPackets(attempt, r, p.Policy.BackupTimeout, p.Policy.VideoTimeout, p.Policy.AudioTimeout)
			sample := make([]byte, 188*7)
			n, e := io.ReadFull(r, sample)
			if e != nil || n < 188 || sample[0] != 0x47 || !timer.Stop() {
				r.Close()
				stop()
				continue
			}
			// The probe context must survive handoff; only the channel lifetime owns it.
			restored := &prefixReader{Reader: io.MultiReader(bytes.NewReader(sample[:n]), r), Closer: &recoveryCloser{ReadCloser: r, cancel: stop}}
			select {
			case out <- restored:
				s.mu.Lock()
				s.activeInput[id] = in.Key
				s.mu.Unlock()
				return
			case <-ctx.Done():
				restored.Close()
				return
			}
		}
	}
}

type recoveryCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
}

func (r *recoveryCloser) Close() error { r.cancel(); return r.ReadCloser.Close() }

func (s *Service) OpenRecording(id string) (*os.File, error) {
	if !localID.MatchString(id) {
		return nil, ErrInvalid
	}
	if _, ok := s.source(id); !ok {
		return nil, ErrInvalid
	}
	dir := filepath.Join(s.directory, "recordings")
	if security.PrivateDirectory(dir, false) != nil {
		return nil, ErrUnavailable
	}
	path := filepath.Join(dir, id+".ts")
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Size() > 32<<20 {
		return nil, ErrUnavailable
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, ErrUnavailable
	}
	return f, nil
}
