package live

import (
	"kore-edge/internal/multicast"
	"os/exec"
	"runtime"
)

type Capabilities struct {
	Tools            Tools                 `json:"tools"`
	Interfaces       []multicast.Interface `json:"interfaces"`
	Platform         string                `json:"platform"`
	SRTSuggestedPort int                   `json:"srt_suggested_port"`
}

func (m *Manager) Capabilities() (Capabilities, error) {
	all, e := m.interfaces()
	if e != nil {
		return Capabilities{}, ErrUnavailable
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	ffprobe, _ := exec.LookPath("ffprobe")
	if !trustedTool(ffmpeg, "ffmpeg") {
		ffmpeg = ""
	}
	if !trustedTool(ffprobe, "ffprobe") {
		ffprobe = ""
	}
	return Capabilities{Tools: Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}, Interfaces: all, Platform: runtime.GOOS, SRTSuggestedPort: 20000}, nil
}
