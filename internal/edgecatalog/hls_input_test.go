package edgecatalog

import (
	"bytes"
	"context"
	"io"
	"kore-edge/internal/hls"
	"kore-edge/internal/wireguard"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPrivateHLSInputNeverAcceptsExternalOrOtherChannelReferences(t *testing.T) {
	token := strings.Repeat("a", 64)
	base := "http://127.0.0.1:12345/private/"
	input := []byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"/api/kore-edge/media/channel/hls/r/" + token + "\"\n/api/kore-edge/media/channel/hls/r/" + token + "\n")
	output, e := privatePlaylist(input, "channel", base)
	if e != nil || strings.Count(string(output), base) != 2 {
		t.Fatal("private reference rewrite", e)
	}
	for _, bad := range []string{"https://external.invalid/private", "file:///etc/passwd", "/api/kore-edge/media/other/hls/index.m3u8", "/api/kore-edge/media/channel/hls/../../file"} {
		if _, e = privatePlaylist([]byte("#EXTM3U\n"+bad+"\n"), "channel", base); e == nil {
			t.Fatal("unsafe reference accepted")
		}
	}
}

func TestSyntheticHLSRemuxWithOpaqueSegmentPaths(t *testing.T) {
	tool := os.Getenv("KORE_TEST_FFMPEG")
	if tool == "" {
		tool, _ = exec.LookPath("ffmpeg")
	}
	if tool == "" {
		t.Skip("ffmpeg unavailable; set KORE_TEST_FFMPEG")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	sample, e := exec.CommandContext(ctx, tool, hls.SyntheticArgs(false)...).Output()
	if e != nil {
		t.Fatal("synthetic generation", e)
	}
	s, _, _ := fixture(t)
	document := manifest()
	document.Channels[0].Protocols = []Protocol{{ID: "hls", Label: "HLS", Path: "/api/kore-edge/media/channel-one/hls/index.m3u8", Verification: "not_tested"}}
	s.data.Catalog = &document
	token := strings.Repeat("a", 64)
	s.client = func(Config, *wireguard.PeerSettings) *http.Client {
		return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			data := sample
			mime := "video/mp2t"
			if strings.HasSuffix(r.URL.Path, "index.m3u8") {
				mime = "application/vnd.apple.mpegurl"
				data = []byte("#EXTM3U\n#EXT-X-TARGETDURATION:2\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:2,\n/api/kore-edge/media/channel-one/hls/r/" + token + "\n#EXT-X-ENDLIST\n")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{mime}}, Body: io.NopCloser(bytes.NewReader(data))}, nil
		})}
	}
	input, e := s.OpenHLS(ctx, "channel-one", tool)
	if e != nil {
		t.Fatal(e)
	}
	defer input.Close()
	packet := make([]byte, 188*7)
	if _, e = io.ReadFull(input, packet); e != nil || packet[0] != 0x47 {
		t.Fatalf("remux failed: %v", e)
	}
}
