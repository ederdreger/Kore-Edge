package live

import (
	"kore-edge/internal/multicast"
	"os/exec"
	"runtime"
)

type Capabilities struct {
	Tools               Tools                 `json:"tools"`
	Interfaces          []multicast.Interface `json:"interfaces"`
	Platform            string                `json:"platform"`
	SRTSuggestedPort    int                   `json:"srt_suggested_port"`
	InterfacesAvailable bool                  `json:"interfaces_available"`
	Warning             string                `json:"warning,omitempty"`
}

func (m *Manager) Capabilities() (Capabilities, error) {
	all, e := m.interfaces()
	available := e == nil
	warning := ""
	if !available {
		all = []multicast.Interface{}
		warning = "Inventário de interfaces indisponível. Entradas HTTP, HLS e SRT continuam configuráveis; UDP exige inventário válido."
	}
	if available {
		for _, nic := range all {
			if nic.IPv4PrimaryOnly {
				warning = "Inventário local compatível com o isolamento do serviço; mostra o IPv4 primário de cada interface. Confirme interface e IP antes de enviar."
				break
			}
		}
	}
	ffmpeg, _ := exec.LookPath("ffmpeg")
	ffprobe, _ := exec.LookPath("ffprobe")
	if !trustedTool(ffmpeg, "ffmpeg") {
		ffmpeg = ""
	}
	if !trustedTool(ffprobe, "ffprobe") {
		ffprobe = ""
	}
	return Capabilities{Tools: Tools{FFmpeg: ffmpeg, FFprobe: ffprobe}, Interfaces: all, InterfacesAvailable: available, Warning: warning, Platform: runtime.GOOS, SRTSuggestedPort: 20000}, nil
}
