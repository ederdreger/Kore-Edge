package manualchannels

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
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

func TestWindowProgressRecoversWithoutWaitingForLifetimeAverage(t *testing.T) {
	p := newEncodingProgress()
	now := time.Unix(100, 0)
	p.clock = func() time.Time { return now }
	p.Write([]byte("frame=100\nout_time_us=4000000\nfps=10\nspeed=0.4x\nprogress=continue\n"))
	if p.snapshot().WindowSpeed != nil {
		t.Fatal("first sample invented a rate")
	}
	now = now.Add(time.Second)
	p.Write([]byte("frame=125\nout_time_us=5000000\nprogress=continue\n"))
	v := p.snapshot()
	if v.WindowSpeed == nil || *v.WindowSpeed != 1 || v.WindowFPS == nil || *v.WindowFPS != 25 || *v.Speed != 0.4 {
		t.Fatal(v)
	}
	*v.WindowFPS = 1000
	if *p.snapshot().WindowFPS != 25 {
		t.Fatal("snapshot aliases live state")
	}
}
