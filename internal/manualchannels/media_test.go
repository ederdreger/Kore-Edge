package manualchannels

import (
	"bytes"
	"context"
	srt "github.com/datarhei/gosrt"
	"io"
	"kore-edge/internal/hls"
	"kore-edge/internal/live"
	"kore-edge/internal/multicast"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestFlussonicSRTStreamIDPlainAndEncryptedSyntheticMedia(t *testing.T) {
	for _, encrypted := range []bool{false, true} {
		name := "plain"
		if encrypted {
			name = "encrypted"
		}
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
			defer cancel()
			config := srt.DefaultConfig()
			config.EnforcedEncryption = encrypted
			config.ReceiverLatency = 20 * time.Millisecond
			config.PeerLatency = 20 * time.Millisecond
			config.PBKeylen = 32
			listener, e := srt.Listen("srt", "127.0.0.1:0", config)
			if e != nil {
				t.Fatal(e)
			}
			defer listener.Close()
			secret := "synthetic-passphrase"
			streamID := "#!::r=SYNTHETIC,m=request"
			accepted := make(chan bool, 1)
			go func() {
				request, e := listener.Accept2()
				if e != nil {
					return
				}
				if request.StreamId() != streamID || request.IsEncrypted() != encrypted {
					request.Reject(srt.REJX_FORBIDDEN)
					accepted <- false
					return
				}
				if encrypted && request.SetPassphrase(secret) != nil {
					request.Reject(srt.REJX_FORBIDDEN)
					accepted <- false
					return
				}
				conn, e := request.Accept()
				if e != nil {
					return
				}
				defer conn.Close()
				accepted <- true
				go func() { <-ctx.Done(); conn.Close() }()
				for {
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Millisecond):
						if _, e = conn.Write(bytes.Repeat(packet(), 7)); e != nil {
							return
						}
					}
				}
			}()
			s := fixture(t)
			s.lookup = func(context.Context, string) ([]net.IPAddr, error) {
				return []net.IPAddr{{IP: net.ParseIP("10.77.0.1")}}, nil
			}
			s.dialSRT = func(network, endpoint string, cfg srt.Config) (srt.Conn, error) {
				if !strings.HasPrefix(endpoint, "10.77.0.1:") || cfg.StreamId != streamID || cfg.Latency != -1 || cfg.ReceiverLatency != 30*time.Millisecond || cfg.PeerLatency != 20*time.Millisecond {
					t.Error("SRT destination or stream ID changed")
				}
				return srt.Dial(network, listener.Addr().String(), cfg)
			}
			_, port, _ := net.SplitHostPort(listener.Addr().String())
			raw := "srt://synthetic.example:" + port + "?latency=20000&rcvlatency=30000&streamid=#!::r=SYNTHETIC,m=request"
			if encrypted {
				raw = "srt://synthetic.example:" + port + "?passphrase=" + secret + "&pbkeylen=32&latency=20000&rcvlatency=30000&streamid=#!::r=SYNTHETIC,m=request"
			}
			id, e := s.Save(Source{URL: raw, Enabled: true}, 0)
			if e != nil {
				t.Fatal(e)
			}
			reader, e := s.OpenSRT(ctx, id)
			if e != nil {
				t.Fatal(e)
			}
			defer reader.Close()
			data := make([]byte, 1316)
			if _, e = io.ReadFull(reader, data); e != nil || !transport(data) {
				t.Fatal("SRT media not received", e)
			}
			select {
			case ok := <-accepted:
				if !ok {
					t.Fatal("stream identity rejected")
				}
			default:
				t.Fatal("no accepted SRT receiver")
			}
		})
	}
}

func TestNativeHLSRemuxUsesPrivateAuthenticatedReferences(t *testing.T) {
	tool := os.Getenv("KORE_MANUAL_FFMPEG")
	if tool == "" {
		t.Skip("set KORE_MANUAL_FFMPEG to run real synthetic HLS remux")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 18*time.Second)
	defer cancel()
	sample, e := exec.CommandContext(ctx, tool, hls.SyntheticArgs(false)...).Output()
	if e != nil {
		t.Fatal("synthetic generation", e)
	}
	s := fixture(t)
	var diagnostic bytes.Buffer
	s.command = func(ctx context.Context, tool string, args ...string) *exec.Cmd {
		for i, a := range args {
			if a == "quiet" {
				args[i] = "error"
			}
			if strings.Contains(a, "synthetic-password") || strings.Contains(a, "media.example") {
				t.Error("origin credential or address reached process arguments")
			}
		}
		cmd := exec.CommandContext(ctx, tool, args...)
		cmd.Stderr = &diagnostic
		return cmd
	}
	var requests atomic.Int32
	s.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		requests.Add(1)
		user, password, ok := r.BasicAuth()
		if !ok || user != "synthetic-user" || password != "synthetic-password" {
			t.Error("same-origin Basic credential lost")
		}
		if r.URL.Host != "media.example" {
			t.Error("origin changed")
		}
		switch r.URL.Path {
		case "/input/index.m3u8":
			return response([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1000000\nvariant.m3u8\n"), "application/vnd.apple.mpegurl"), nil
		case "/input/variant.m3u8":
			return response([]byte("#EXTM3U\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:0\n#EXTINF:6,\nsegment.ts\n#EXT-X-ENDLIST\n"), "application/vnd.apple.mpegurl"), nil
		case "/input/segment.ts":
			return response(sample, "video/mp2t"), nil
		default:
			t.Error("unexpected HLS path")
			return nil, ErrDenied
		}
	})}
	for _, entry := range []string{"index.m3u8", "variant.m3u8"} {
		diagnostic.Reset()
		before := requests.Load()
		id, e := s.Save(Source{URL: "https://synthetic-user:synthetic-password@media.example/input/" + entry, Enabled: true}, s.Revision())
		if e != nil {
			t.Fatal(e)
		}
		reader, e := s.OpenHLS(ctx, id, tool)
		if e != nil {
			t.Fatal(e)
		}
		defer reader.Close()
		data := make([]byte, 1316)
		if _, e = io.ReadFull(reader, data); e != nil || !transport(data) {
			reader.Close()
			safe := strings.ReplaceAll(diagnostic.String(), "synthetic-password", "[credential]")
			safe = regexp.MustCompile(`https?://[^\s'"\]]+`).ReplaceAllString(safe, "[private-relay]")
			t.Fatal("HLS remux failed", e, safe)
		}
		reader.Close()
		if requests.Load()-before < 3 {
			t.Fatal("variant/segment not fetched")
		}
	}
}

func TestManualSourceUsesOneContinuousIngestAndExistingPlayer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	s := fixture(t)
	var opens atomic.Int32
	s.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		opens.Add(1)
		reader, writer := io.Pipe()
		go func() {
			defer writer.Close()
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-r.Context().Done():
					return
				case <-ticker.C:
					if _, e := writer.Write(bytes.Repeat(packet(), 7)); e != nil {
						return
					}
				}
			}
		}()
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/mp2t"}}, Body: reader}, nil
	})}
	id, e := s.Save(Source{Name: "Synthetic", URL: "https://media.example/channel/mpegts", Enabled: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	m, e := live.New(ctx, filepath.Join(t.TempDir(), "live"), s)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if m.Save(live.ChannelConfig{ChannelID: id, Protocol: "http_ts"}, live.Tools{}, 0) != nil || m.StartReviewed(id, 1) != nil {
		t.Fatal("manual continuous setup")
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		snap := m.Snapshot()
		if len(snap.Streams) == 1 && snap.Streams[0].Metrics.Bytes > 1316 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	before := m.Snapshot().Streams[0].Metrics.Bytes
	time.Sleep(50 * time.Millisecond)
	if m.Snapshot().Streams[0].Metrics.Bytes <= before || opens.Load() != 1 {
		t.Fatal("source did not advance without a player")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.ServeTS(w, r, id) }))
	defer server.Close()
	request, _ := http.NewRequestWithContext(ctx, "GET", server.URL, nil)
	response, e := http.DefaultClient.Do(request)
	if e != nil {
		t.Fatal(e)
	}
	if response.StatusCode != 200 {
		response.Body.Close()
		t.Fatal("continuous preview", response.StatusCode)
	}
	data := make([]byte, 1316)
	_, e = io.ReadFull(response.Body, data)
	response.Body.Close()
	if e != nil || !transport(data) || opens.Load() != 1 {
		t.Fatal("preview reopened origin or lost media")
	}
	if m.Stop(id) != nil {
		t.Fatal("stop")
	}
	if s.Inventory().Revision != 1 || m.Snapshot().ManualRevision != 1 {
		t.Fatal("manual revision missing")
	}
}

func TestUDPUnicastSocketOpensOnlyAfterExplicitStart(t *testing.T) {
	s := fixture(t)
	s.interfaces = func() ([]multicast.Interface, error) {
		return []multicast.Interface{{Name: "synthetic-loopback", Up: true, IPv4: []string{"127.0.0.1"}}}, nil
	}
	probe, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if e != nil {
		t.Fatal(e)
	}
	port := probe.LocalAddr().(*net.UDPAddr).Port
	probe.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, e := s.openUDP(ctx, Source{URL: "udp://127.0.0.1:" + strconv.Itoa(port), Interface: "synthetic-loopback", LocalIP: "127.0.0.1"})
	if e != nil {
		t.Fatal(e)
	}
	defer reader.Close()
	sender, e := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: port})
	if e != nil {
		t.Fatal(e)
	}
	defer sender.Close()
	sender.Write(bytes.Repeat(packet(), 7))
	data := make([]byte, 564)
	if _, e = io.ReadFull(reader, data); e != nil || !transport(data) {
		t.Fatal("UDP receive", e)
	}
}
