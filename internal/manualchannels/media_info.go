package manualchannels

import (
	"bytes"
	"context"
	"encoding/json"
	srt "github.com/datarhei/gosrt"
	"io"
	"kore-edge/internal/mediaresources"
	"kore-edge/internal/traffic"
	"kore-edge/internal/tsmetrics"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type MediaTrack struct {
	Kind       string  `json:"kind"`
	Codec      string  `json:"codec"`
	Index      int     `json:"index"`
	PID        *uint64 `json:"pid,omitempty"`
	Width      int     `json:"width,omitempty"`
	Height     int     `json:"height,omitempty"`
	Channels   int     `json:"channels,omitempty"`
	SampleRate uint64  `json:"sample_rate,omitempty"`
	Bitrate    uint64  `json:"bits_per_second,omitempty"`
	Language   string  `json:"language,omitempty"`
}

// MediaInfo describes sampled bytes, never the requested encoder settings.
type MediaInfo struct {
	Tracks           []MediaTrack `json:"tracks,omitempty"`
	ContinuityErrors uint64       `json:"continuity_errors"`
	TransportErrors  uint64       `json:"transport_errors"`
	Video            string       `json:"video,omitempty"`
	Audio            string       `json:"audio,omitempty"`
	Width            int          `json:"width,omitempty"`
	Height           int          `json:"height,omitempty"`
	MeasuredAt       time.Time    `json:"measured_at"`
}
type SRTMediaStats struct {
	Received      uint64  `json:"received"`
	Missing       uint64  `json:"missing"`
	Retransmitted uint64  `json:"retransmitted"`
	Dropped       uint64  `json:"dropped"`
	Undecryptable uint64  `json:"undecryptable"`
	RTTMS         float64 `json:"rtt_ms"`
	LatencyMS     uint64  `json:"latency_ms"`
}
type pipelineMedia struct {
	input, output *MediaInfo
	srt           func() *SRTMediaStats
	receiver      string
	inputRate     *traffic.Counter
}

func srtMediaStats(reader io.ReadCloser) func() *SRTMediaStats {
	if wrapper, ok := reader.(*cancelReader); ok {
		reader = wrapper.ReadCloser
	}
	connection, ok := reader.(interface{ Stats(*srt.Statistics) })
	if !ok {
		return nil
	}
	return func() *SRTMediaStats {
		var stats srt.Statistics
		connection.Stats(&stats)
		a := stats.Accumulated
		return &SRTMediaStats{Received: a.PktRecv, Missing: a.PktRecvLoss, Retransmitted: a.PktRecvRetrans, Dropped: a.PktRecvDrop, Undecryptable: a.PktRecvUndecrypt, RTTMS: stats.Instantaneous.MsRTT, LatencyMS: stats.Instantaneous.MsRecvTsbPdDelay}
	}
}
func sampleTransport(info *MediaInfo, sample []byte) {
	now := time.Now().UTC()
	analyzer := tsmetrics.New(now)
	for offset := 0; offset+188 <= len(sample); {
		if sample[offset] != 0x47 {
			offset++
			continue
		}
		analyzer.Observe(sample[offset:offset+188], now)
		offset += 188
	}
	stats := analyzer.Snapshot()
	info.ContinuityErrors = stats.CCErrors
	info.TransportErrors = stats.TEI
}

// Bound both retained samples and ffprobe processes independently of channels.
var mediaProbeSlots = make(chan struct{}, 2)
var safeCodec = regexp.MustCompile(`^[a-zA-Z0-9_]{1,32}$`)

func decodeMedia(data []byte) *MediaInfo {
	var document struct {
		Streams []struct {
			Type       string `json:"codec_type"`
			Codec      string `json:"codec_name"`
			Width      int    `json:"width"`
			Height     int    `json:"height"`
			Index      int    `json:"index"`
			ID         string `json:"id"`
			Channels   int    `json:"channels"`
			SampleRate string `json:"sample_rate"`
			Bitrate    string `json:"bit_rate"`
			Tags       struct {
				Language string `json:"language"`
			} `json:"tags"`
		} `json:"streams"`
	}
	if json.Unmarshal(data, &document) != nil {
		return nil
	}
	info := &MediaInfo{MeasuredAt: time.Now().UTC()}
	for _, stream := range document.Streams {
		if !safeCodec.MatchString(stream.Codec) {
			continue
		}
		if stream.Type == "video" || stream.Type == "audio" || stream.Type == "subtitle" {
			track := MediaTrack{Kind: stream.Type, Codec: stream.Codec, Index: stream.Index}
			if pid, err := strconv.ParseUint(stream.ID, 0, 16); err == nil && pid < 8192 {
				track.PID = &pid
			}
			if stream.Width > 0 && stream.Width <= 16384 && stream.Height > 0 && stream.Height <= 16384 {
				track.Width, track.Height = stream.Width, stream.Height
			}
			if stream.Channels > 0 && stream.Channels <= 32 {
				track.Channels = stream.Channels
			}
			if value, err := strconv.ParseUint(stream.SampleRate, 10, 64); err == nil && value >= 8000 && value <= 384000 {
				track.SampleRate = value
			}
			if value, err := strconv.ParseUint(stream.Bitrate, 10, 64); err == nil && value > 0 && value <= 1000000000 {
				track.Bitrate = value
			}
			if regexp.MustCompile(`^[a-zA-Z]{2,3}$`).MatchString(stream.Tags.Language) {
				track.Language = strings.ToLower(stream.Tags.Language)
			}
			info.Tracks = append(info.Tracks, track)
		}
		if stream.Type == "audio" && info.Audio == "" {
			info.Audio = stream.Codec
		}
		if stream.Type == "video" && info.Video == "" && stream.Width > 0 && stream.Width <= 16384 && stream.Height > 0 && stream.Height <= 16384 {
			info.Video, info.Width, info.Height = stream.Codec, stream.Width, stream.Height
		}
	}
	if info.Video == "" && info.Audio == "" {
		return nil
	}
	return info
}

type probeBuffer struct{ bytes.Buffer }

func (b *probeBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16384 {
		return 0, io.ErrShortBuffer
	}
	return b.Buffer.Write(p)
}

type mediaObserver struct {
	io.ReadCloser
	ctx          context.Context
	path         string
	record       func(*MediaInfo)
	sample       []byte
	start, retry time.Time
	release      func()
	done         bool
	mu           sync.Mutex
	closed       bool
	rate         *traffic.Counter
}

func observeMedia(ctx context.Context, input io.ReadCloser, path string, record func(*MediaInfo), counters ...*traffic.Counter) io.ReadCloser {
	if path == "" && len(counters) == 0 {
		return input
	}
	var counter *traffic.Counter
	if len(counters) > 0 {
		counter = counters[0]
	}
	return &mediaObserver{ReadCloser: input, ctx: ctx, path: path, record: record, rate: counter}
}

func (r *mediaObserver) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	if r.rate != nil {
		r.rate.Add(n)
	}
	if r.path == "" {
		return n, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if (n == 0 && (err != io.EOF || len(r.sample) == 0)) || r.closed || r.done || r.ctx.Err() != nil {
		return n, err
	}
	if r.release == nil {
		if time.Now().Before(r.retry) {
			return n, err
		}
		select {
		case mediaProbeSlots <- struct{}{}:
			var once sync.Once
			r.release = func() { once.Do(func() { <-mediaProbeSlots }) }
			r.start = time.Now()
		default:
			r.retry = time.Now().Add(5 * time.Second)
			return n, err
		}
	}
	remaining := 1024*1024 - len(r.sample)
	if remaining > n {
		remaining = n
	}
	r.sample = append(r.sample, p[:remaining]...)
	if len(r.sample) >= 1024*1024 || time.Since(r.start) >= 3*time.Second || err == io.EOF {
		r.done = true
		sample := r.sample
		r.sample = nil
		runCtx, cancel := context.WithTimeout(r.ctx, 6*time.Second)
		release := r.release
		go func() {
			defer cancel()
			defer release()
			cmd := exec.CommandContext(runCtx, r.path, "-v", "quiet", "-protocol_whitelist", "pipe", "-f", "mpegts", "-show_entries", "stream=index,id,codec_type,codec_name,width,height,channels,sample_rate,bit_rate:stream_tags=language", "-of", "json", "pipe:0")
			mediaresources.Configure(cmd)
			cmd.WaitDelay = time.Second
			cmd.Stdin = bytes.NewReader(sample)
			out := &probeBuffer{}
			cmd.Stdout = out
			if cmd.Run() == nil && runCtx.Err() == nil {
				if info := decodeMedia(out.Bytes()); info != nil {
					sampleTransport(info, sample)
					r.record(info)
				}
			}
		}()
	}
	return n, err
}
func (r *mediaObserver) Close() error {
	err := r.ReadCloser.Close()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.closed = true
	if !r.done && r.release != nil {
		r.release()
		r.sample = nil
	}
	return err
}

func (s *Service) InputBitrate(id string) (*float64, bool) {
	s.mu.Lock()
	media := s.mediaInfo[id]
	s.mu.Unlock()
	if media == nil || media.inputRate == nil {
		return nil, false
	}
	return media.inputRate.Rate(), true
}
