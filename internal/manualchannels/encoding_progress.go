package manualchannels

import (
	"math"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Only numeric FFmpeg progress and exit status leave this parser.
// Raw stderr can contain source metadata and is never logged or persisted.
type EncodingSnapshot struct {
	State       string    `json:"state"`
	Frames      uint64    `json:"frames"`
	FPS         *float64  `json:"fps"`
	Speed       *float64  `json:"speed"`
	WindowFPS   *float64  `json:"window_fps"`
	WindowSpeed *float64  `json:"window_speed"`
	UpdatedAt   time.Time `json:"updated_at"`
	ExitCode    *int      `json:"exit_code,omitempty"`
}
type encodingProgress struct {
	mu             sync.Mutex
	line           []byte
	discard        bool
	value          EncodingSnapshot
	clock          func() time.Time
	previousAt     time.Time
	previousFrames uint64
	outputUS       int64
	previousUS     int64
}

func newEncodingProgress() *encodingProgress {
	return &encodingProgress{value: EncodingSnapshot{State: "starting"}, clock: time.Now}
}
func (p *encodingProgress) Write(data []byte) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, b := range data {
		if b == '\n' {
			if !p.discard {
				p.parse(string(p.line))
			}
			p.line = p.line[:0]
			p.discard = false
			continue
		}
		if len(p.line) >= 4096 {
			p.discard = true
			continue
		}
		p.line = append(p.line, b)
	}
	return len(data), nil
}
func (p *encodingProgress) parse(line string) {
	key, value, ok := strings.Cut(strings.TrimSpace(line), "=")
	if !ok {
		return
	}
	switch key {
	case "out_time_us":
		if n, e := strconv.ParseInt(strings.TrimSpace(value), 10, 64); e == nil && n >= 0 {
			p.outputUS = n
		}
	case "frame":
		if n, e := strconv.ParseUint(strings.TrimSpace(value), 10, 64); e == nil {
			p.value.Frames = n
		}
	case "fps", "speed":
		n, e := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), "x"), 64)
		if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 10000 {
			return
		}
		if key == "fps" {
			p.value.FPS = &n
		} else {
			p.value.Speed = &n
		}
	case "progress":
		if value == "continue" {
			p.value.State = "running"
			now := p.clock()
			elapsed := now.Sub(p.previousAt).Seconds()
			if !p.previousAt.IsZero() && elapsed >= 0.25 && p.value.Frames >= p.previousFrames && p.outputUS >= p.previousUS {
				fps := float64(p.value.Frames-p.previousFrames) / elapsed
				speed := float64(p.outputUS-p.previousUS) / 1e6 / elapsed
				if fps <= 10000 && speed <= 10000 {
					p.value.WindowFPS, p.value.WindowSpeed = &fps, &speed
				} else {
					p.value.WindowFPS, p.value.WindowSpeed = nil, nil
				}
			} else {
				p.value.WindowFPS, p.value.WindowSpeed = nil, nil
			}
			p.previousAt, p.previousFrames, p.previousUS = now, p.value.Frames, p.outputUS
			p.value.UpdatedAt = now.UTC()
		}
	}
}
func (p *encodingProgress) finish(err error, cancelled bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	code := 0
	p.value.State = "exited"
	if err != nil {
		code = -1
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		}
		p.value.State = "failed"
	}
	if cancelled {
		p.value.State = "stopped"
	}
	p.value.ExitCode = &code
	p.value.UpdatedAt = time.Now().UTC()
}
func (p *encodingProgress) snapshot() *EncodingSnapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	v := p.value
	if v.WindowFPS != nil {
		n := *v.WindowFPS
		v.WindowFPS = &n
	}
	if v.WindowSpeed != nil {
		n := *v.WindowSpeed
		v.WindowSpeed = &n
	}
	if v.FPS != nil {
		n := *v.FPS
		v.FPS = &n
	}
	if v.Speed != nil {
		n := *v.Speed
		v.Speed = &n
	}
	if v.ExitCode != nil {
		n := *v.ExitCode
		v.ExitCode = &n
	}
	return &v
}
