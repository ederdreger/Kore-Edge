package manualchannels

import (
	"bytes"
	"context"
	srt "github.com/datarhei/gosrt"
	"io"
	"kore-edge/internal/hls"
	"kore-edge/internal/mediaresources"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSRTManifestKeepsOptionsPrivateAndRejectsInjection(t *testing.T) {
	value, err := srtManifest("srt://192.0.2.1:12000", map[string]string{"srt_streamid": "#!::r=SYNTHETIC,m=request", "passphrase": "synthetic '& key"})
	if err != nil || !strings.Contains(value, "option srt_streamid '#!::r=SYNTHETIC,m=request'") || !strings.Contains(value, "'\\''") {
		t.Fatal("options were not escaped")
	}
	for _, options := range []map[string]string{{"srt_streamid": "x\nfile /etc/passwd"}, {"passphrase": "secret\x00suffix"}, {"protocol_whitelist": "file"}, {"mode\nfile": "caller"}} {
		if _, err := srtManifest("srt://192.0.2.1:12000", options); err == nil {
			t.Fatal("unsafe manifest accepted")
		}
	}
}

func TestNativeSRTRejectsReboundDestinationBeforeSpawning(t *testing.T) {
	s := fixture(t)
	s.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
	}
	s.command = func(context.Context, string, ...string) *exec.Cmd {
		t.Fatal("spawned for a denied destination")
		return nil
	}
	if _, err := s.openNativeSRT(context.Background(), Source{URL: "srt://synthetic.example:12000?streamid=synthetic"}, "unused"); err == nil {
		t.Fatal("rebound destination accepted")
	}
}

func TestNativeSRTReceiverPreservesStreamIdentityAndCleansUp(t *testing.T) {
	tool := os.Getenv("KORE_MANUAL_FFMPEG")
	if tool == "" {
		t.Skip("set KORE_MANUAL_FFMPEG for synthetic native SRT interoperability")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	sample, err := exec.CommandContext(ctx, tool, hls.SyntheticArgs(false)...).Output()
	if err != nil {
		t.Fatal("synthetic TS generation", err)
	}
	for _, encrypted := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "encrypted"}[encrypted], func(t *testing.T) {
			listener, err := srt.Listen("srt", "127.0.0.1:0", srt.DefaultConfig())
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			streamID := "#!::r=SYNTHETIC,m=request"
			secret := "synthetic '& key"
			child, stop := context.WithCancel(ctx)
			defer stop()
			accepted := make(chan bool, 1)
			go func() {
				request, err := listener.Accept2()
				if err != nil {
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
				conn, err := request.Accept()
				if err != nil {
					return
				}
				defer conn.Close()
				accepted <- true
				go func() { <-child.Done(); conn.Close() }()
				for {
					for offset := 0; offset < len(sample); offset += 1316 {
						end := offset + 1316
						if end > len(sample) {
							end = len(sample)
						}
						select {
						case <-child.Done():
							return
						case <-time.After(2 * time.Millisecond):
						}
						if _, err = conn.Write(sample[offset:end]); err != nil {
							return
						}
					}
				}
			}()
			options := map[string]string{"mode": "caller", "transtype": "live", "latency": "200000", "srt_streamid": streamID}
			if encrypted {
				options["passphrase"] = secret
				options["pbkeylen"] = "32"
			}
			manifest, err := srtManifest("srt://"+listener.Addr().String(), options)
			if err != nil {
				t.Fatal("manifest")
			}
			s := fixture(t)
			s.command = func(ctx context.Context, path string, args ...string) *exec.Cmd {
				joined := strings.Join(args, " ")
				for _, private := range []string{secret, streamID, listener.Addr().String()} {
					if strings.Contains(joined, private) {
						t.Error("private SRT value leaked into process arguments")
					}
				}
				return exec.CommandContext(ctx, path, args...)
			}
			reader, err := s.startNativeSRT(child, tool, manifest)
			if err != nil {
				t.Fatal("start native receiver", err)
			}
			defer reader.Close()
			data := make([]byte, 188*7)
			if _, err = io.ReadFull(reader, data); err != nil || !transport(data) {
				t.Fatal("native SRT did not return TS", err)
			}
			select {
			case ok := <-accepted:
				if !ok {
					t.Fatal("identity/encryption mismatch")
				}
			default:
				t.Fatal("no accepted receiver")
			}
			if bytes.Contains(data, []byte(secret)) {
				t.Fatal("private option entered media")
			}
			closed := make(chan struct{})
			go func() { reader.Close(); reader.Close(); close(closed) }()
			select {
			case <-closed:
			case <-time.After(3 * time.Second):
				t.Fatal("native receiver did not close")
			}
			first, ok := mediaresources.Input()
			if !ok {
				t.Fatal("receiver slot leaked")
			}
			defer first()
			second, ok := mediaresources.Input()
			if !ok {
				t.Fatal("receiver slot leaked")
			}
			second()
		})
	}
}
