package manualchannels

import (
	"context"
	"io"
	"kore-edge/internal/mediaresources"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type EncoderDevice struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}
type EncoderCapabilities struct {
	Devices       []EncoderDevice `json:"devices"`
	MaxConcurrent int             `json:"max_concurrent"`
	Warning       string          `json:"warning"`
}
type encoderCache struct {
	mu    sync.Mutex
	at    time.Time
	path  string
	value EncoderCapabilities
}

func deviceArgs(device string) []string {
	if strings.HasPrefix(device, "vaapi:") {
		return []string{"-vaapi_device", "/dev/dri/" + strings.TrimPrefix(device, "vaapi:")}
	}
	return nil
}
func encoderArgs(device string) []string {
	if strings.HasPrefix(device, "nvidia:") {
		return []string{"-c:v", "h264_nvenc", "-gpu", strings.TrimPrefix(device, "nvidia:"), "-preset", "p4"}
	}
	if strings.HasPrefix(device, "vaapi:") {
		return []string{"-c:v", "h264_vaapi"}
	}
	return []string{"-c:v", "libx264", "-preset", "veryfast", "-threads:v", "1"}
}
func videoFilter(device string, w, h int) string {
	result := "scale=" + strconv.Itoa(w) + ":" + strconv.Itoa(h) + ":force_original_aspect_ratio=decrease,pad=" + strconv.Itoa(w) + ":" + strconv.Itoa(h) + ":(ow-iw)/2:(oh-ih)/2,setsar=1,format=yuv420p"
	if strings.HasPrefix(device, "vaapi:") {
		result += ",format=nv12,hwupload"
	}
	return result
}

// Each candidate must actually encode a synthetic frame as the service user.
// Merely finding a GPU or an encoder compiled into FFmpeg is insufficient.
func (s *Service) Encoders(ctx context.Context, ffmpeg string) EncoderCapabilities {
	s.encoders.mu.Lock()
	defer s.encoders.mu.Unlock()
	if s.encoders.path == ffmpeg && time.Since(s.encoders.at) < time.Minute {
		return s.encoders.value
	}
	result := EncoderCapabilities{Devices: []EncoderDevice{}, MaxConcurrent: cap(s.transcodeSlots), Warning: "Sem transcodificar preserva o fluxo original. Processamento utiliza recursos e exige codecs compatíveis."}
	if executable(ffmpeg) {
		candidates := []EncoderDevice{{"cpu", "Processador (CPU) · H.264"}}
		paths, _ := filepath.Glob("/dev/nvidia[0-9]*")
		for _, p := range paths {
			v := strings.TrimPrefix(filepath.Base(p), "nvidia")
			if regexp.MustCompile(`^[0-9]{1,2}$`).MatchString(v) {
				candidates = append(candidates, EncoderDevice{"nvidia:" + v, "NVIDIA GPU " + v + " · H.264 NVENC"})
			}
		}
		paths, _ = filepath.Glob("/dev/dri/renderD[0-9]*")
		for _, p := range paths {
			v := filepath.Base(p)
			if regexp.MustCompile(`^renderD[0-9]{3}$`).MatchString(v) {
				candidates = append(candidates, EncoderDevice{"vaapi:" + v, "GPU VAAPI " + v + " · H.264"})
			}
		}
		if len(candidates) > 9 {
			candidates = candidates[:9]
		}
		for _, d := range candidates {
			if ctx.Err() != nil {
				break
			}
			child, cancel := context.WithTimeout(ctx, 2*time.Second)
			args := append([]string{"-hide_banner", "-loglevel", "quiet", "-nostdin"}, deviceArgs(d.ID)...)
			args = append(args, "-f", "lavfi", "-i", "color=size=320x180:rate=25", "-vf", videoFilter(d.ID, 320, 180))
			args = append(args, encoderArgs(d.ID)...)
			args = append(args, "-frames:v", "1", "-f", "null", "-")
			cmd := s.command(child, ffmpeg, args...)
			mediaresources.Configure(cmd)
			cmd.Stdout = io.Discard
			cmd.Stderr = io.Discard
			cmd.WaitDelay = time.Second
			err := cmd.Run()
			cancel()
			if err == nil {
				result.Devices = append(result.Devices, d)
			}
		}
	}
	if ctx.Err() == nil {
		s.encoders.at = time.Now()
		s.encoders.path = ffmpeg
		s.encoders.value = result
	}
	return result
}
func (s *Service) CanEncode(ctx context.Context, ffmpeg, device string) bool {
	for _, d := range s.Encoders(ctx, ffmpeg).Devices {
		if d.ID == device {
			return true
		}
	}
	return false
}

type encodedInput struct {
	reader *io.PipeReader
	input  io.ReadCloser
	cancel context.CancelFunc
	done   chan struct{}
	once   sync.Once
}

func (p *encodedInput) Read(b []byte) (int, error) { return p.reader.Read(b) }
func (p *encodedInput) Close() error {
	p.once.Do(func() { p.cancel(); p.input.Close(); p.reader.Close(); <-p.done })
	return nil
}
func transcodeArgs(t Transcode) []string { return configuredTranscodeArgs(t) }
func (s *Service) reserveEncoder() (func(), error) {
	select {
	case s.transcodeSlots <- struct{}{}:
		var once sync.Once
		return func() { once.Do(func() { <-s.transcodeSlots }) }, nil
	default:
		return nil, mediaresources.ErrTranscodeBusy
	}
}
func (s *Service) transcode(parent context.Context, input io.ReadCloser, ffmpeg string, t Transcode) (io.ReadCloser, error) {
	release, err := s.reserveEncoder()
	if err != nil {
		input.Close()
		return nil, err
	}
	result, err := s.encodeReserved(parent, input, ffmpeg, t, release)
	if err != nil {
		release()
	}
	return result, err
}
func (s *Service) encodeReserved(parent context.Context, input io.ReadCloser, ffmpeg string, t Transcode, release func(), progress ...*encodingProgress) (io.ReadCloser, error) {
	if !executable(ffmpeg) || !validateTranscode(t) {
		input.Close()
		return nil, ErrUnavailable
	}
	t.logoPaths = map[string]string{}
	for _, track := range t.Tracks {
		if track.Logo != "" {
			path, e := s.assetPath(track.Logo, "logo")
			if e != nil {
				input.Close()
				return nil, e
			}
			t.logoPaths[track.Logo] = path
		}
	}
	return s.runEncoder(parent, input, ffmpeg, configuredTranscodeArgs(t, s.encoderThreads), release, true, progress...)
}

// An opt-in reserve needs a single remuxer to normalize transport continuity
// across input switches; codecs remain untouched in copy mode.
func (s *Service) copyReserved(parent context.Context, input io.ReadCloser, ffmpeg string, release func()) (io.ReadCloser, error) {
	if !executable(ffmpeg) {
		input.Close()
		return nil, ErrUnavailable
	}
	args := []string{"-hide_banner", "-loglevel", "quiet", "-nostdin", "-protocol_whitelist", "pipe", "-fflags", "+genpts", "-dts_delta_threshold", "0.1", "-f", "mpegts", "-i", "pipe:0", "-map", "0:v:0", "-map", "0:a?", "-c", "copy", "-mpegts_flags", "+resend_headers+initial_discontinuity", "-flush_packets", "1", "-f", "mpegts", "pipe:1"}
	return s.runEncoder(parent, input, ffmpeg, args, release, false)
}
func (s *Service) runEncoder(parent context.Context, input io.ReadCloser, ffmpeg string, args []string, release func(), encoding bool, progress ...*encodingProgress) (io.ReadCloser, error) {
	ctx, cancel := context.WithCancel(parent)
	var stats *encodingProgress
	if encoding && len(progress) > 0 {
		stats = progress[0]
		args = append([]string{"-progress", "pipe:2", "-stats_period", "1", "-nostats"}, args...)
	}
	cmd := s.command(ctx, ffmpeg, args...)
	if encoding {
		mediaresources.ConfigureEncoding(cmd)
	} else {
		mediaresources.Configure(cmd)
	}
	cmd.Stdin = input
	cmd.Stderr = io.Discard
	if stats != nil {
		cmd.Stderr = stats
	}
	cmd.WaitDelay = time.Second
	reader, writer := io.Pipe()
	cmd.Stdout = writer
	if cmd.Start() != nil {
		if stats != nil {
			stats.finish(ErrUnavailable, false)
		}
		cancel()
		input.Close()
		reader.Close()
		writer.Close()
		return nil, ErrUnavailable
	}
	p := &encodedInput{reader: reader, input: input, cancel: cancel, done: make(chan struct{})}
	go func() {
		err := cmd.Wait()
		if stats != nil {
			stats.finish(err, ctx.Err() != nil)
		}
		cancel()
		input.Close()
		if err != nil {
			writer.CloseWithError(ErrUnavailable)
		} else {
			writer.Close()
		}
		release()
		close(p.done)
	}()
	go func() {
		select {
		case <-parent.Done():
			input.Close()
			reader.CloseWithError(ErrUnavailable)
		case <-p.done:
		}
	}()
	return p, nil
}
