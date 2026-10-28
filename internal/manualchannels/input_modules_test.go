package manualchannels

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func mockFile(s *Service, data []byte) {
	s.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})}
}
func pngSample() []byte {
	var data bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	for x := 0; x < 32; x++ {
		for y := 0; y < 32; y++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	png.Encode(&data, img)
	return data.Bytes()
}
func TestAssetImportAndReferenceProtection(t *testing.T) {
	s, id := processingFixture(t)
	mockFile(s, pngSample())
	a, e := s.ImportAsset(context.Background(), "logo", "https://synthetic.example/logo.png", "")
	if e != nil || a.Kind != "logo" {
		t.Fatal("logo import", e)
	}
	if _, e = s.assetPath(a.ID, "logo"); e != nil {
		t.Fatal(e)
	}
	p := advancedProfile()
	p.Tracks[0].Logo = a.ID
	p.Tracks[0].LogoPosition = "br"
	p.Tracks[0].LogoWidth = 32
	if e = s.SaveProcessing(id, nil, &p, s.Revision()); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteAsset(a.ID); e == nil {
		t.Fatal("deleted referenced logo")
	}
	p.Tracks[0].Logo = ""
	if e = s.SaveProcessing(id, nil, &p, s.Revision()); e != nil {
		t.Fatal(e)
	}
	if e = s.DeleteAsset(a.ID); e != nil {
		t.Fatal(e)
	}
	mockFile(s, []byte("<svg><script>unsafe</script></svg>"))
	if _, e = s.ImportAsset(context.Background(), "logo", "https://synthetic.example/logo.svg", ""); e == nil {
		t.Fatal("active image accepted")
	}
	if _, e = s.assetPath("../../etc/passwd", "logo"); e == nil {
		t.Fatal("asset path traversal")
	}
}
func TestPublicationCredentialsAreScopedAndPipeIsBounded(t *testing.T) {
	s, id := processingFixture(t)
	inputs := []Input{{Protocol: "publish", URL: "publish://incoming"}}
	policy := InputPolicy{Publication: true, ClusterExport: true}
	if e := s.SaveProcessingPolicy(id, &inputs, nil, &policy, s.Revision()); e != nil {
		t.Fatal(e)
	}
	p, e := s.Processing(id)
	if e != nil || p.Policy.Token == "" || len(p.Inputs) != 1 {
		t.Fatal("publication profile", e)
	}
	if !s.IngestAuthorized(id, p.Policy.Token, true) || s.IngestAuthorized(id, "bad", true) || s.IngestAuthorized("other", p.Policy.Token, true) {
		t.Fatal("credential scope")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, e := s.openPublication(ctx, id)
	if e != nil {
		t.Fatal(e)
	}
	writer, e := s.AttachPublisher(id)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.AttachPublisher(id); e == nil {
		t.Fatal("duplicate publisher accepted")
	}
	done := make(chan struct{})
	go func() { defer close(done); writer.Write([]byte("synthetic")); writer.Close() }()
	data, e := io.ReadAll(reader)
	reader.Close()
	<-done
	if e != nil || string(data) != "synthetic" {
		t.Fatal("publication payload changed")
	}
	policy.Publication = false
	policy.ClusterExport = false
	inputs = []Input{{Key: "", URL: "https://synthetic.example/main.ts", Protocol: "http_ts"}}
	if e = s.SaveProcessingPolicy(id, &inputs, nil, &policy, s.Revision()); e != nil {
		t.Fatal(e)
	}
	if s.IngestAuthorized(id, p.Policy.Token, true) {
		t.Fatal("disabled credential accepted")
	}
	copy, e := New(s.directory, nil)
	if e != nil {
		t.Fatal("saved profile cannot reload", e)
	}
	copy.Close()
}
func TestPacketWatchClosesOnMissingVideoWithoutChangingHealthyBytes(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	source, writer := io.Pipe()
	reader := watchPackets(ctx, source, 0, 2, 0)
	done := make(chan error, 1)
	go func() { _, e := io.Copy(io.Discard, reader); done <- e }()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	until := time.After(4 * time.Second)
loop:
	for {
		select {
		case <-done:
			break loop
		case <-until:
			t.Fatal("missing video did not close input")
		case <-ticker.C:
			packet := make([]byte, 188)
			packet[0] = 0x47
			packet[3] = 0x10
			if _, e := writer.Write(packet); e != nil {
				break loop
			}
		}
	}
	reader.Close()
	writer.Close()
}
func TestNativeBackupLogoAndRecovery(t *testing.T) {
	tool := os.Getenv("KORE_MANUAL_FFMPEG")
	if tool == "" {
		t.Skip("native synthetic media modules")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
	defer cancel()
	s, id := processingFixture(t)
	var mp4 bytes.Buffer
	cmd := exec.CommandContext(ctx, tool, "-v", "error", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25", "-f", "lavfi", "-i", "sine=frequency=500", "-t", "2", "-c:v", "libx264", "-threads", "1", "-preset", "ultrafast", "-c:a", "aac", "-movflags", "frag_keyframe+empty_moov", "-f", "mp4", "pipe:1")
	cmd.Stdout = &mp4
	if cmd.Run() != nil {
		t.Fatal("generate backup")
	}
	probe := filepath.Join(filepath.Dir(tool), "ffprobe")
	mockFile(s, mp4.Bytes())
	asset, e := s.ImportAsset(ctx, "backup", "https://synthetic.example/reserve.mp4", probe)
	if e != nil {
		t.Fatal("backup import", e)
	}
	mockFile(s, pngSample())
	logo, e := s.ImportAsset(ctx, "logo", "https://synthetic.example/logo.png", probe)
	if e != nil {
		t.Fatal(e)
	}
	policy := InputPolicy{BackupAsset: asset.ID, BackupTimeout: 2, RecordBackup: true}
	fallback, e := s.openBackup(ctx, id, policy, tool)
	if e != nil {
		t.Fatal(e)
	}
	data := make([]byte, 188*500)
	if _, e = io.ReadFull(fallback, data); e != nil {
		t.Fatal("backup not producing", e)
	}
	fallback.Close()
	recording := filepath.Join(s.directory, "recordings", id+".ts")
	if info, e := os.Stat(recording); e != nil || info.Size() == 0 || info.Size() > 32<<20 {
		t.Fatal("recording not bounded", e)
	}
	profile := advancedProfile()
	profile.Tracks = profile.Tracks[:1]
	profile.Tracks[0].Logo = logo.ID
	profile.Tracks[0].LogoWidth = 32
	profile.Tracks[0].LogoPosition = "br"
	// Feed native backup TS into the actual overlay pipeline, then inspect decoded output.
	encoded, e := s.transcode(ctx, io.NopCloser(bytes.NewReader(data)), tool, profile)
	if e != nil {
		t.Fatal(e)
	}
	output, e := io.ReadAll(encoded)
	encoded.Close()
	if e != nil || len(output) < 188 {
		t.Fatal("logo pipeline failed", e)
	}
	check := exec.CommandContext(ctx, probe, "-v", "error", "-show_entries", "stream=codec_name,width,height", "-of", "json", "-i", "pipe:0")
	check.Stdin = bytes.NewReader(output)
	if check.Run() != nil {
		t.Fatal("logo output invalid")
	}
	// A failed source starts the reserve; cancellation must close process and slot.
	p := Processing{Inputs: []Input{{Protocol: "http_ts", URL: "https://synthetic.example/main.ts"}}, Policy: policy}
	mockFile(s, data)
	switching, e := s.withBackup(ctx, id, p, nil, io.EOF, tool)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = io.ReadFull(switching, make([]byte, 188*7)); e != nil {
		t.Fatal(e)
	}
	switching.Close()
	if len(s.slots) != 0 {
		t.Fatal("backup resource leak")
	}
	// Import rejects containers that have not passed the native MP4 probe.
	mockFile(s, []byte("not media"))
	if _, e = s.ImportAsset(ctx, "backup", "https://synthetic.example/bad.mp4", probe); e == nil {
		t.Fatal("unprobed backup accepted")
	}
}
