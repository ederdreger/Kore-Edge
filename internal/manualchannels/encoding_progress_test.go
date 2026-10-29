package manualchannels

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEncodingProgressIsBoundedAndNeverExportsRawStderr(t *testing.T) {
	p := newEncodingProgress()
	p.Write([]byte("https://synthetic.invalid/private?token=SECRET\nframe=250\nfps=25.4\nspe"))
	p.Write([]byte("ed=1.02x\nprogress=continue\n"))
	v := p.snapshot()
	if v.Frames != 250 || v.FPS == nil || *v.FPS != 25.4 || v.Speed == nil || *v.Speed != 1.02 || v.State != "running" {
		t.Fatal("progress unavailable")
	}
	p.Write([]byte("fps=NaN\nspeed=Inf\n" + strings.Repeat("SECRET", 2000) + "\n"))
	data, e := json.Marshal(p.snapshot())
	if e != nil || strings.Contains(string(data), "SECRET") {
		t.Fatal("invalid value or secret escaped progress parser")
	}
	p.finish(nil, false)
	if p.snapshot().ExitCode == nil || *p.snapshot().ExitCode != 0 {
		t.Fatal("missing exit status")
	}
}
