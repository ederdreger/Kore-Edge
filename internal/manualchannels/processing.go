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
	"os"
	"path/filepath"
	"reflect"
	"regexp"
)

// Processing is separate from the legacy origin file so old releases can roll back.
// Input URLs are write-only; inventory returns stable keys and sanitized addresses.
type Input struct {
	Key       string `json:"key"`
	URL       string `json:"url,omitempty"`
	Protocol  string `json:"protocol"`
	Interface string `json:"interface,omitempty"`
	LocalIP   string `json:"local_ip,omitempty"`
}
type Transcode struct {
	Mode      string `json:"mode"`
	Device    string `json:"device,omitempty"`
	VideoKbps int    `json:"video_kbps,omitempty"`
	Width     int    `json:"width,omitempty"`
	Height    int    `json:"height,omitempty"`
	FPS       int    `json:"fps,omitempty"`
	Audio     string `json:"audio,omitempty"`
	AudioKbps int    `json:"audio_kbps,omitempty"`
}
type Processing struct {
	Base      string    `json:"base"`
	Inputs    []Input   `json:"inputs"`
	Transcode Transcode `json:"transcode"`
}
type processingState struct {
	Version  int                   `json:"version"`
	Revision uint64                `json:"revision"`
	Entries  map[string]Processing `json:"entries"`
}
type InputView struct {
	Key       string `json:"key"`
	Address   string `json:"address"`
	Protocol  string `json:"protocol"`
	Interface string `json:"interface"`
	LocalIP   string `json:"local_ip"`
}
type ProcessingView struct {
	Revision  uint64      `json:"revision"`
	Inputs    []InputView `json:"inputs"`
	Transcode Transcode   `json:"transcode"`
	ActiveKey string      `json:"active_key"`
	Local     bool        `json:"local"`
}

var processingID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var inputKey = regexp.MustCompile(`^(primary|[a-f0-9]{48})$`)

func fingerprint(c Source) string {
	b, _ := json.Marshal([]string{c.URL, c.Protocol, c.Interface, c.LocalIP})
	hash := sha256.Sum256(b)
	return hex.EncodeToString(hash[:])
}
func validateTranscode(t Transcode) bool {
	if t.Mode == "copy" {
		return t.Device == "" && t.VideoKbps == 0 && t.Width == 0 && t.Height == 0 && t.FPS == 0 && t.Audio == "" && t.AudioKbps == 0
	}
	return t.Mode == "h264" && (t.Device == "cpu" || regexp.MustCompile(`^(nvidia:[0-9]{1,2}|vaapi:renderD[0-9]{3})$`).MatchString(t.Device)) && t.VideoKbps >= 200 && t.VideoKbps <= 30000 && t.Width >= 160 && t.Width <= 3840 && t.Width%2 == 0 && t.Height >= 90 && t.Height <= 2160 && t.Height%2 == 0 && t.FPS >= 1 && t.FPS <= 60 && (t.Audio == "copy" || t.Audio == "aac" && t.AudioKbps >= 32 && t.AudioKbps <= 320)
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
		if !processingID.MatchString(id) || !validateTranscode(p.Transcode) || len(p.Inputs) > 8 || localID.MatchString(id) && (len(p.Inputs) < 1 || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(p.Base)) || !localID.MatchString(id) && (len(p.Inputs) != 0 || p.Base != "") {
			return ErrUnavailable
		}
		keys := map[string]bool{}
		for _, in := range p.Inputs {
			if !inputKey.MatchString(in.Key) || keys[in.Key] {
				return ErrUnavailable
			}
			keys[in.Key] = true
			if _, e := normalize(Source{Name: "Input", URL: in.URL, Protocol: in.Protocol, Interface: in.Interface, LocalIP: in.LocalIP}); e != nil {
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
	out := ProcessingView{Revision: s.revisionLocked(), Inputs: []InputView{}, Transcode: p.Transcode, ActiveKey: s.activeInput[id], Local: ok}
	for _, in := range p.Inputs {
		u, _ := parse(in.URL)
		out.Inputs = append(out.Inputs, InputView{in.Key, safeAddress(u), in.Protocol, in.Interface, in.LocalIP})
	}
	return out, nil
}

// SaveProcessing validates all inputs before writing. Existing secrets are retained by key.
// A nil inputs argument edits only processing; nil transcode edits only inputs.
func (s *Service) SaveProcessing(id string, inputs *[]Input, transcode *Transcode, expected uint64) error {
	c, local := s.source(id)
	if !processingID.MatchString(id) || !local && inputs != nil {
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
			}
			normalized, e := normalize(Source{Name: "Input", URL: v.URL, Protocol: v.Protocol, Interface: v.Interface, LocalIP: v.LocalIP})
			if e != nil {
				return e
			}
			if normalized.Protocol == "udp" {
				if _, e = s.udpTarget(normalized); e != nil {
					return e
				}
			}
			v.URL = normalized.URL
			v.Protocol = normalized.Protocol
			next = append(next, v)
		}
		p.Inputs = next
	}
	if transcode != nil {
		if !validateTranscode(*transcode) {
			return ErrInvalid
		}
		p.Transcode = *transcode
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
	return nil
}

// OpenConfigured is used only when a reviewed profile exists. The legacy path
// remains unchanged for every channel without such a profile.
func (s *Service) OpenConfigured(ctx context.Context, id, protocol, ffmpeg string) (io.ReadCloser, bool, error) {
	c, local := s.source(id)
	s.mu.Lock()
	stored, exists := s.processing.Entries[id]
	p := s.processingLocked(id, c)
	handled := exists && (local && stored.Base == fingerprint(c) || !local && stored.Base == "")
	start := s.nextInput[id]
	s.mu.Unlock()
	if !handled {
		return nil, false, nil
	}
	if !s.Authorized(id, protocol) {
		return nil, true, ErrDenied
	}
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
			candidate := Source{URL: in.URL, Protocol: in.Protocol, Interface: in.Interface, LocalIP: in.LocalIP}
			switch in.Protocol {
			case "http_ts":
				var responseBody io.ReadCloser
				response, e := s.openHTTP(ctx, in.URL, "")
				err = e
				if e == nil {
					responseBody = response.Body
				}
				reader = responseBody
			case "hls":
				reader, err = s.openHLS(ctx, candidate, ffmpeg)
			case "srt":
				reader, err = s.openSRT(ctx, candidate)
			case "udp":
				reader, err = s.openUDP(ctx, candidate)
			}
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
	if err != nil {
		return nil, true, err
	}
	if reader == nil {
		return nil, true, ErrUnavailable
	}
	if p.Transcode.Mode == "copy" {
		return reader, true, nil
	}
	result, e := s.transcode(ctx, reader, ffmpeg, p.Transcode)
	return result, true, e
}
