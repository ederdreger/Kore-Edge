package manualchannels

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"kore-edge/internal/catalog"
	"kore-edge/internal/mediaresources"
	"kore-edge/internal/security"
	"kore-edge/internal/traffic"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
)

// Processing is separate from the legacy origin file so old releases can roll back.
// Input URLs are write-only; inventory returns stable keys and sanitized addresses.
type Input struct {
	Key           string `json:"key"`
	URL           string `json:"url,omitempty"`
	Protocol      string `json:"protocol"`
	Interface     string `json:"interface,omitempty"`
	LocalIP       string `json:"local_ip,omitempty"`
	SourceTimeout int    `json:"source_timeout,omitempty"`
	Token         string `json:"token,omitempty"`
	Certificate   string `json:"certificate,omitempty"`
}
type Transcode struct {
	logoPaths     map[string]string
	Mode          string       `json:"mode"`
	Device        string       `json:"device,omitempty"`
	VideoKbps     int          `json:"video_kbps,omitempty"`
	Width         int          `json:"width,omitempty"`
	Height        int          `json:"height,omitempty"`
	FPS           int          `json:"fps,omitempty"`
	Audio         string       `json:"audio,omitempty"`
	AudioKbps     int          `json:"audio_kbps,omitempty"`
	VideoCopy     bool         `json:"video_copy,omitempty"`
	SampleRate    int          `json:"sample_rate,omitempty"`
	Channels      int          `json:"channels,omitempty"`
	VolumeDB      float64      `json:"volume_db,omitempty"`
	SplitChannels bool         `json:"split_channels,omitempty"`
	Deinterlace   string       `json:"deinterlace,omitempty"`
	Crop          string       `json:"crop,omitempty"`
	GOP           int          `json:"gop,omitempty"`
	PixelFormat   string       `json:"pixel_format,omitempty"`
	Tracks        []VideoTrack `json:"tracks,omitempty"`
}
type Processing struct {
	Base      string      `json:"base"`
	Inputs    []Input     `json:"inputs"`
	Transcode Transcode   `json:"transcode"`
	Policy    InputPolicy `json:"policy,omitempty"`
}
type processingState struct {
	Version  int                   `json:"version"`
	Revision uint64                `json:"revision"`
	Entries  map[string]Processing `json:"entries"`
}
type InputView struct {
	Key           string `json:"key"`
	Address       string `json:"address"`
	Protocol      string `json:"protocol"`
	Interface     string `json:"interface"`
	LocalIP       string `json:"local_ip"`
	SourceTimeout int    `json:"source_timeout,omitempty"`
	Certificate   string `json:"certificate,omitempty"`
}
type ProcessingView struct {
	Revision      uint64         `json:"revision"`
	Inputs        []InputView    `json:"inputs"`
	Transcode     Transcode      `json:"transcode"`
	ActiveKey     string         `json:"active_key"`
	Local         bool           `json:"local"`
	InputReceiver string         `json:"input_receiver,omitempty"`
	InputSRT      *SRTMediaStats `json:"input_srt,omitempty"`
	InputMedia    *MediaInfo     `json:"input_media,omitempty"`
	OutputMedia   *MediaInfo     `json:"output_media,omitempty"`
	Policy        InputPolicy    `json:"policy"`
	BackupActive  bool           `json:"backup_active"`
}

var processingID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var inputKey = regexp.MustCompile(`^(primary|[a-f0-9]{48})$`)

func fingerprint(c Source) string {
	b, _ := json.Marshal([]string{c.URL, c.Protocol, c.Interface, c.LocalIP})
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}
func validateTranscode(t Transcode) bool {
	return validTranscode(t)
}
func (s *Service) loadProcessing() error {
	s.processing = processingState{Version: 1, Entries: map[string]Processing{}}
	s.nextInput = map[string]int{}
	s.activeInput = map[string]string{}
	file := filepath.Join(s.directory, "processing.json")
	if _, e := os.Lstat(file); os.IsNotExist(e) {
		return nil
	}
	data, e := security.ReadPrivateFile(file, 4*1024*1024)
	if e != nil || catalog.DecodeStrict(data, &s.processing) != nil || s.processing.Version != 1 || s.processing.Entries == nil || len(s.processing.Entries) > 512 {
		return ErrUnavailable
	}
	for id, p := range s.processing.Entries {
		if !validPolicy(p.Policy) || !processingID.MatchString(id) || !validateTranscode(p.Transcode) || len(p.Inputs) > 8 || localID.MatchString(id) && (len(p.Inputs) < 1 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(p.Base)) || !localID.MatchString(id) && (len(p.Inputs) != 0 || p.Base != "") {
			return ErrUnavailable
		}
		keys := map[string]bool{}
		for _, in := range p.Inputs {
			if in.SourceTimeout != 0 && (in.SourceTimeout < 2 || in.SourceTimeout > 300) {
				return ErrUnavailable
			}
			if !inputKey.MatchString(in.Key) || keys[in.Key] {
				return ErrUnavailable
			}
			keys[in.Key] = true
			if _, e := normalizeInput(in); e != nil {
				return ErrUnavailable
			}
		}
	}
	return nil
}
func (s *Service) processingLocked(id string, c Source) Processing {
	if p, ok := s.processing.Entries[id]; ok && (c.ID == "" && p.Base == "" || c.ID != "" && p.Base == fingerprint(c)) {
		p.Inputs = append([]Input{}, p.Inputs...)
		return p
	}
	p := Processing{Transcode: Transcode{Mode: "copy"}, Inputs: []Input{}}
	if c.ID != "" {
		p.Base = fingerprint(c)
		p.Inputs = []Input{{Key: "primary", URL: c.URL, Protocol: c.Protocol, Interface: c.Interface, LocalIP: c.LocalIP}}
	}
	return p
}
func (s *Service) Processing(id string) (ProcessingView, error) {
	c, ok := s.source(id)
	if !ok {
		if s.IsManual(id) || s.fallback == nil {
			return ProcessingView{}, ErrInvalid
		}
		found := false
		v := s.fallback.Snapshot()
		if v.Catalog != nil {
			for _, ch := range v.Catalog.Channels {
				if ch.ID == id {
					found = true
				}
			}
		}
		if !found {
			return ProcessingView{}, ErrInvalid
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.processingLocked(id, c)
	out := ProcessingView{Revision: s.revisionLocked(), Inputs: []InputView{}, Transcode: p.Transcode, ActiveKey: s.activeInput[id], Local: ok, Policy: p.Policy}
	if media := s.mediaInfo[id]; media != nil {
		out.BackupActive = media.backup
		out.InputMedia, out.OutputMedia = media.input, media.output
		out.InputReceiver = media.receiver
		if media.srt != nil {
			out.InputSRT = media.srt()
		}
	}
	for _, in := range p.Inputs {
		address := "publish://incoming"
		if in.Protocol != "publish" {
			u, e := parse(in.URL)
			if e != nil {
				return ProcessingView{}, ErrUnavailable
			}
			address = safeAddress(u)
		}
		out.Inputs = append(out.Inputs, InputView{in.Key, address, in.Protocol, in.Interface, in.LocalIP, in.SourceTimeout, in.Certificate})
	}
	return out, nil
}

// SaveProcessing validates all inputs before writing. Existing secrets are retained by key.
// A nil inputs argument edits only processing; nil transcode edits only inputs.
func (s *Service) SaveProcessing(id string, inputs *[]Input, transcode *Transcode, expected uint64) error {
	return s.SaveProcessingPolicy(id, inputs, transcode, nil, expected)
}
func (s *Service) SaveProcessingPolicy(id string, inputs *[]Input, transcode *Transcode, policy *InputPolicy, expected uint64) error {
	c, local := s.source(id)
	if !processingID.MatchString(id) || !local && (inputs != nil || policy != nil) {
		return ErrInvalid
	}
	if _, e := s.Processing(id); e != nil {
		return e
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected != s.revisionLocked() {
		return ErrConflict
	}
	p := s.processingLocked(id, c)
	previous := p
	if inputs != nil {
		if len(*inputs) < 1 || len(*inputs) > 8 {
			return ErrInvalid
		}
		byKey := map[string]Input{}
		for _, v := range p.Inputs {
			byKey[v.Key] = v
		}
		next := []Input{}
		seen := map[string]bool{}
		for _, v := range *inputs {
			if v.SourceTimeout != 0 && (v.SourceTimeout < 2 || v.SourceTimeout > 300) {
				return ErrInvalid
			}
			if v.Key == "" {
				v.Key = random()
			}
			if !inputKey.MatchString(v.Key) || seen[v.Key] {
				return ErrInvalid
			}
			seen[v.Key] = true
			if v.URL == "" {
				old, ok := byKey[v.Key]
				if !ok {
					return ErrInvalid
				}
				v.URL = old.URL
				if v.Token == "" && v.Protocol == "cluster" {
					v.Token = old.Token
				}
			}
			normalized, e := normalizeInput(v)
			if e != nil {
				return e
			}
			if normalized.Protocol == "udp" {
				if _, e = s.udpTarget(Source{URL: normalized.URL, Protocol: normalized.Protocol, Interface: normalized.Interface, LocalIP: normalized.LocalIP}); e != nil {
					return e
				}
			}
			v.URL = normalized.URL
			v.Protocol = normalized.Protocol
			next = append(next, v)
		}
		p.Inputs = next
	}
	if policy != nil {
		if !validPolicy(*policy) {
			return ErrInvalid
		}
		nextPolicy := *policy
		nextPolicy.Token = p.Policy.Token
		if (nextPolicy.Publication || nextPolicy.ClusterExport) && (nextPolicy.Token == "" || nextPolicy.RotateToken) {
			nextPolicy.Token = random()
			if !inputKey.MatchString(nextPolicy.Token) || nextPolicy.Token == "primary" {
				return ErrUnavailable
			}
		}
		nextPolicy.RotateToken = false
		if nextPolicy.BackupAsset != "" {
			if _, e := s.assetPath(nextPolicy.BackupAsset, "backup"); e != nil {
				return e
			}
		}
		p.Policy = nextPolicy
		if p.Policy.BackupAsset != "" && p.Policy.BackupTimeout == 0 {
			p.Policy.BackupTimeout = 5
		}
	}
	for _, in := range p.Inputs {
		if in.Protocol == "publish" && !p.Policy.Publication {
			return ErrInvalid
		}
		if in.Protocol == "cluster" && p.Policy.ClusterExport {
			return ErrInvalid
		}
	}
	if transcode != nil {
		if !validateTranscode(*transcode) {
			return ErrInvalid
		}
		for _, v := range transcode.Tracks {
			if v.Logo != "" {
				if _, e := s.assetPath(v.Logo, "logo"); e != nil {
					return e
				}
			}
		}
		p.Transcode = *transcode
		if transcode.Mode != "copy" && p.Policy.BackupAsset != "" {
			p.Policy.BackupTranscode = true
		}
	}
	if p.Policy.BackupAsset != "" && p.Transcode.Mode != "copy" && !p.Policy.BackupTranscode {
		return ErrInvalid
	}
	if reflect.DeepEqual(previous, p) {
		return nil
	}
	next := processingState{Version: 1, Revision: s.processing.Revision + 1, Entries: map[string]Processing{}}
	for key, v := range s.processing.Entries {
		if key == id {
			continue
		}
		exists := false
		for _, source := range s.data.Sources {
			if source.ID == key {
				exists = true
			}
		}
		if exists || !s.IsManual(key) {
			next.Entries[key] = v
		}
	}
	if len(next.Entries) >= 512 {
		return ErrConflict
	}
	next.Entries[id] = p
	if e := s.persistFile("processing.json", next); e != nil {
		return e
	}
	s.processing = next
	delete(s.nextInput, id)
	delete(s.activeInput, id)
	delete(s.mediaInfo, id)
	return nil
}

// OpenConfigured uses the reviewed profile or the local source in copy mode.
// Sources without a local registration or processing profile use the legacy path.
func (s *Service) OpenConfigured(ctx context.Context, id, protocol, ffmpeg string) (io.ReadCloser, bool, error) {
	return s.OpenConfiguredMedia(ctx, id, protocol, ffmpeg, "")
}

// SourceIdleTimeout overrides only the live no-payload watchdog for the selected input.
// Zero retains the established receiver defaults.
func (s *Service) SourceIdleTimeout(id string) int {
	c, ok := s.source(id)
	if !ok {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.processingLocked(id, c)
	if p.Policy.BackupAsset != "" {
		if media := s.mediaInfo[id]; media != nil && media.backup {
			return 0
		}
		seconds := p.Policy.BackupTimeout
		if seconds == 0 {
			seconds = 5
		}
		for _, in := range p.Inputs {
			if in.Key == s.activeInput[id] && in.SourceTimeout > 0 && in.SourceTimeout < seconds {
				seconds = in.SourceTimeout
			}
		}
		if seconds > 297 {
			return 300
		}
		return seconds + 3
	}
	for _, in := range p.Inputs {
		if in.Key == s.activeInput[id] {
			return in.SourceTimeout
		}
	}
	return 0
}

func (s *Service) OpenConfiguredMedia(ctx context.Context, id, protocol, ffmpeg, ffprobe string) (io.ReadCloser, bool, error) {
	c, local := s.source(id)
	s.mu.Lock()
	stored, exists := s.processing.Entries[id]
	p := s.processingLocked(id, c)
	handled := local || exists && !local && stored.Base == ""
	start := s.nextInput[id]
	s.mu.Unlock()
	if !handled {
		return nil, false, nil
	}
	if !s.Authorized(id, protocol) {
		return nil, true, ErrDenied
	}
	release := func() {}
	retained := false
	if p.Transcode.Mode != "copy" || p.Policy.BackupAsset != "" {
		var reserveError error
		release, reserveError = s.reserveEncoder()
		if reserveError != nil {
			return nil, true, reserveError
		}
	}
	defer func() {
		if !retained {
			release()
		}
	}()
	var reader io.ReadCloser
	var err error
	if local {
		// One origin at a time. A failed/idle attempt rotates; healthy streams stay open.
		for n := 0; n < len(p.Inputs); n++ {
			if ctx.Err() != nil {
				return nil, true, ctx.Err()
			}
			i := (start + n) % len(p.Inputs)
			in := p.Inputs[i]
			reader, err = s.openInput(ctx, id, in, ffmpeg)
			if errors.Is(err, mediaresources.ErrBusy) {
				return nil, true, err
			}
			if err == nil {
				s.mu.Lock()
				s.nextInput[id] = (i + 1) % len(p.Inputs)
				s.activeInput[id] = in.Key
				s.mu.Unlock()
				break
			}
			if p.Policy.BackupAsset != "" {
				break
			}
		}
	} else {
		switch protocol {
		case "http_ts":
			reader, err = s.fallback.OpenHTTP(ctx, id)
		case "hls":
			reader, err = s.fallback.OpenHLS(ctx, id, ffmpeg)
		case "srt":
			reader, err = s.fallback.OpenSRT(ctx, id)
		default:
			err = ErrInvalid
		}
	}
	if local && reader != nil {
		reader = watchPackets(ctx, reader, p.Policy.BackupTimeout, p.Policy.VideoTimeout, p.Policy.AudioTimeout)
	}
	if local && p.Policy.BackupAsset != "" {
		reader, err = s.withBackup(ctx, id, p, reader, err, ffmpeg)
	}
	if err != nil {
		return nil, true, err
	}
	if reader == nil {
		return nil, true, ErrUnavailable
	}
	media := &pipelineMedia{srt: srtMediaStats(reader), inputRate: traffic.New()}
	if named, ok := reader.(interface{ ReceiverName() string }); ok {
		media.receiver = named.ReceiverName()
	} else if media.srt != nil {
		media.receiver = "GoSRT"
	}
	s.mu.Lock()
	if s.mediaInfo == nil {
		s.mediaInfo = map[string]*pipelineMedia{}
	}
	s.mediaInfo[id] = media
	s.mu.Unlock()
	record := func(input bool) func(*MediaInfo) {
		return func(info *MediaInfo) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.mediaInfo[id] != media || ctx.Err() != nil {
				return
			}
			if input {
				media.input = info
				if p.Transcode.Mode == "copy" {
					media.output = info
				}
			} else {
				media.output = info
			}
		}
	}
	reader = observeMedia(ctx, reader, ffprobe, record(true), media.inputRate)
	if p.Transcode.Mode == "copy" {
		if p.Policy.BackupAsset != "" {
			result, e := s.copyReserved(ctx, reader, ffmpeg, release)
			retained = e == nil
			return result, true, e
		}
		return reader, true, nil
	}
	result, e := s.encodeReserved(ctx, reader, ffmpeg, p.Transcode, release)
	retained = e == nil
	if e == nil {
		result = observeMedia(ctx, result, ffprobe, record(false))
	}
	return result, true, e
}
