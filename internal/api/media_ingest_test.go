package api

import (
	"context"
	"io"
	"kore-edge/internal/live"
	"kore-edge/internal/manualchannels"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNativePublication(t *testing.T) {
	tool := os.Getenv("KORE_PLAYBACK_FFMPEG")
	if tool == "" {
		t.Skip("native synthetic publisher")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	source, e := manualchannels.New(filepath.Join(t.TempDir(), "sources"), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer source.Close()
	id, e := source.Save(manualchannels.Source{Name: "Synthetic", URL: "https://synthetic.example/test.ts", Protocol: "http_ts", Enabled: true}, source.Revision())
	if e != nil {
		t.Fatal(e)
	}
	inputs := []manualchannels.Input{{Protocol: "publish", URL: "publish://incoming"}}
	policy := manualchannels.InputPolicy{Publication: true}
	if e = source.SaveProcessingPolicy(id, &inputs, nil, &policy, source.Revision()); e != nil {
		t.Fatal(e)
	}
	profile, _ := source.Processing(id)
	manager, e := live.New(ctx, filepath.Join(t.TempDir(), "live"), apiManualCatalog{source})
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	a := &API{ManualChannels: source, Live: manager, PublicOrigin: "https://edge.example"}
	server := httptest.NewServer(http.HandlerFunc(a.mediaIngest))
	defer server.Close()
	reader, _, e := source.OpenConfigured(ctx, id, "http_ts", "")
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	result := make(chan int64, 1)
	go func() { n, _ := io.Copy(io.Discard, reader); result <- n }()
	cmd := exec.CommandContext(ctx, tool, "-v", "quiet", "-f", "lavfi", "-i", "testsrc2=size=320x180:rate=25", "-t", "2", "-c:v", "libx264", "-threads", "1", "-preset", "ultrafast", "-f", "mpegts", "-method", "POST", "-headers", "Authorization: Bearer "+profile.Policy.Token+"\r\n", server.URL+"/api/v1/publish/"+id+"/stream.ts")
	if cmd.Run() != nil {
		t.Fatal("native HTTP publisher failed")
	}
	select {
	case n := <-result:
		if n < 188*20 {
			t.Fatal("missing published TS")
		}
	case <-ctx.Done():
		t.Fatal("publisher did not close cleanly")
	}
}

func TestMachineIngestHasSeparateCredentialAndNoBrowserBypass(t *testing.T) {
	source, e := manualchannels.New(filepath.Join(t.TempDir(), "sources"), nil)
	if e != nil {
		t.Fatal(e)
	}
	defer source.Close()
	id, e := source.Save(manualchannels.Source{Name: "Synthetic", URL: "https://synthetic.example/test.ts", Protocol: "http_ts", Enabled: true}, source.Revision())
	if e != nil {
		t.Fatal(e)
	}
	inputs := []manualchannels.Input{{Protocol: "publish", URL: "publish://incoming"}}
	policy := manualchannels.InputPolicy{Publication: true, ClusterExport: true}
	if e = source.SaveProcessingPolicy(id, &inputs, nil, &policy, source.Revision()); e != nil {
		t.Fatal(e)
	}
	profile, _ := source.Processing(id)
	manager, e := live.New(context.Background(), filepath.Join(t.TempDir(), "live"), apiManualCatalog{source})
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	a := &API{ManualChannels: source, Live: manager, PublicOrigin: "https://edge.example"}
	call := func(method, path, token, origin, remote string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader("synthetic"))
		r.RemoteAddr = remote
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Origin", origin)
		w := httptest.NewRecorder()
		a.mediaIngest(w, r)
		return w
	}
	path := "/api/v1/publish/" + id + "/stream.ts"
	if call("POST", path, "bad", "", "127.0.0.1:12").Code != 401 {
		t.Fatal("anonymous write accepted")
	}
	if call("POST", path, profile.Policy.Token, "https://other.example", "127.0.0.1:12").Code != 403 {
		t.Fatal("browser request accepted")
	}
	if call("POST", path, profile.Policy.Token, "", "192.0.2.10:12").Code != 403 {
		t.Fatal("gateway boundary bypass")
	}
	if call("POST", path, profile.Policy.Token, "", "127.0.0.1:12").Code != 409 {
		t.Fatal("publication without waiting receiver accepted")
	}
	reader, handled, e := source.OpenConfigured(context.Background(), id, "http_ts", "")
	if !handled || e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	data := make(chan string, 1)
	go func() { b, _ := io.ReadAll(reader); data <- string(b) }()
	if w := call("POST", path, profile.Policy.Token, "", "127.0.0.1:12"); w.Code != http.StatusOK {
		t.Fatal("authorized publisher", w.Code)
	}
	if <-data != "synthetic" {
		t.Fatal("payload changed")
	}
}
