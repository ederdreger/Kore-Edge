package config

import (
	"path/filepath"
	"strings"
	"testing"

	"kore-edge/internal/hls"
)

func TestHLSConfiguration(t *testing.T) {
	c, e := Parse(strings.NewReader(valid))
	if e != nil {
		t.Fatal(e)
	}
	base := t.TempDir()
	c.HLS = hls.Config{Enabled: true, FFmpeg: filepath.Join(base, "ffmpeg"), FFprobe: filepath.Join(base, "ffprobe"), Root: filepath.Join(base, "media")}
	if c.Validate() == nil {
		t.Fatal("HLS without stream")
	}
	c.Stream.Enabled = true
	c.Catalog.PartnerID = "partner-lab"
	c.Catalog.PublicKey = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
	if c.Validate() != nil {
		t.Fatal("valid HLS config")
	}
	c.HLS.Root = "relative"
	if c.Validate() == nil {
		t.Fatal("relative storage")
	}
}
