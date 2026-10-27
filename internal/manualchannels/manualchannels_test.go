package manualchannels

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"kore-edge/internal/edgecatalog"
	"kore-edge/internal/multicast"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func fixture(t *testing.T) *Service {
	t.Helper()
	s, e := New(filepath.Join(t.TempDir(), "manual"), nil)
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(s.Close)
	return s
}
func packet() []byte {
	p := bytes.Repeat([]byte{0xff}, 188)
	p[0] = 0x47
	p[1] = 0x01
	p[2] = 0
	p[3] = 0x10
	return p
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func response(data []byte, mime string) *http.Response {
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{mime}}, Body: io.NopCloser(bytes.NewReader(data))}
}

func TestDetectFlussonicFormatsAndProtectStreamID(t *testing.T) {
	for _, test := range []struct{ url, protocol, name string }{
		{"http://media.example/CHANNEL/mpegts", "http_ts", "CHANNEL"},
		{"https://media.example/CHANNEL/index.m3u8?token=synthetic", "hls", "CHANNEL"},
		{"srt://media.example:12000?streamid=#!::r=CHANNEL,m=request", "srt", "CHANNEL"},
		{"udp://@239.77.0.1:12000", "udp", "239.77.0.1"},
	} {
		d, e := Detect(test.url, "")
		if e != nil || d.Protocol != test.protocol || d.Name != test.name || d.Verified {
			t.Fatalf("detection for %s: %+v %v", test.protocol, d, e)
		}
	}
	u, e := parse("srt://media.example:12000?passphrase=synthetic-secret&streamid=#!::r=CHANNEL,m=request")
	if e != nil || u.Query().Get("streamid") != "#!::r=CHANNEL,m=request" {
		t.Fatal("Flussonic streamid lost")
	}
	for _, bad := range []string{"file:///etc/passwd", "rtsp://media.example/a", "srt://media.example", "srt://media.example:12000?mode=listener", "srt://media.example:12000?passphrase=short", "srt://media.example:12000?mode=caller&mode=listener", "http://127.0.0.1/a.ts", "http://169.254.169.254/a.ts", "https://media.example/a\r\nInjected: yes"} {
		if _, e := Detect(bad, ""); e == nil {
			t.Fatal("invalid source accepted")
		}
	}
}

func TestPrivatePersistenceRevisionAndSecretFreeInventory(t *testing.T) {
	s := fixture(t)
	id, e := s.Save(Source{Name: "Synthetic", URL: "https://user:synthetic-password@media.example/CHANNEL/mpegts?token=synthetic-token", Enabled: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(s.Inventory())
	for _, secret := range []string{"synthetic-password", "synthetic-token", "user:", "token="} {
		if bytes.Contains(b, []byte(secret)) {
			t.Fatal("secret leaked in inventory")
		}
	}
	if _, e = s.Save(Source{ID: id, Name: "Changed", URL: "", Protocol: "http_ts", Enabled: true}, 0); e != ErrConflict {
		t.Fatal("stale revision accepted")
	}
	if _, e = s.Save(Source{ID: id, Name: "Changed", URL: "", Protocol: "http_ts", Enabled: true}, 1); e != nil {
		t.Fatal(e)
	}
	reopened, e := New(s.directory, nil)
	if e != nil {
		t.Fatal(e)
	}
	c, ok := reopened.source(id)
	if !ok || !strings.Contains(c.URL, "synthetic-password") {
		t.Fatal("stored credential not preserved")
	}
	if os.PathSeparator != '\\' {
		info, _ := os.Stat(filepath.Join(s.directory, "channels.json"))
		if info.Mode().Perm() != 0600 {
			t.Fatal("private file permissions")
		}
	}
	if !s.Authorized(id, "http_ts") || s.Authorized(id, "srt") || s.Authorized("remote", "http_ts") {
		t.Fatal("manual permission expanded")
	}
	if e = s.Delete(id, 2); e != nil || s.Authorized(id, "http_ts") {
		t.Fatal("revocation")
	}
	if _, e = New("relative-directory", nil); e == nil {
		t.Fatal("relative private storage accepted")
	}
}

func TestHTTPProbeReadsTransportAndClosesTheOrigin(t *testing.T) {
	s := fixture(t)
	var opens atomic.Int32
	s.client = &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		opens.Add(1)
		if r.URL.Host != "media.example" {
			t.Fatal("unexpected origin")
		}
		return response(bytes.Repeat(packet(), 7), "video/mp2t"), nil
	})}
	d, e := s.Probe(context.Background(), Source{URL: "https://media.example/channel", Enabled: true}, "")
	if e != nil || d.Protocol != "http_ts" || !d.Verified || opens.Load() != 1 {
		t.Fatal("HTTP autodetection", d, e)
	}
	if len(s.Inventory().Channels) != 0 {
		t.Fatal("probe saved a source")
	}
	s.client = &http.Client{Transport: roundTrip(func(*http.Request) (*http.Response, error) {
		return response([]byte("<html>not a stream</html>"), "text/html; charset=utf-8"), nil
	})}
	if _, e = s.Probe(context.Background(), Source{URL: "https://media.example/channel/mpegts"}, ""); e == nil {
		t.Fatal("HTML accepted as media")
	}
}

func TestAddressPinningAndUDPDestinationsRequireExplicitInterfaces(t *testing.T) {
	s := fixture(t)
	s.lookup = func(context.Context, string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.ParseIP("10.77.0.1")}, {IP: net.ParseIP("127.0.0.1")}}, nil
	}
	if _, e := s.resolve(context.Background(), "media.example"); e != ErrDenied {
		t.Fatal("mixed safe/unsafe DNS accepted")
	}
	for _, raw := range []string{"127.0.0.1", "169.254.169.254", "100.100.100.200", "224.0.0.1", "0.0.0.0", "::1", "fe80::1", "fd00:ec2::254"} {
		if permitted(netip.MustParseAddr(raw)) {
			t.Fatal("unsafe address allowed", raw)
		}
	}
	s.interfaces = func() ([]multicast.Interface, error) {
		return []multicast.Interface{{Name: "synthetic0", Up: true, Multicast: true, IPv4: []string{"10.77.0.2"}}}, nil
	}
	base := Source{URL: "udp://239.77.0.1:12000", Protocol: "udp", Enabled: true}
	if _, e := s.Save(base, 0); e == nil {
		t.Fatal("implicit NIC accepted")
	}
	base.Interface = "synthetic0"
	base.LocalIP = "10.77.0.2"
	if _, e := s.Save(base, 0); e != nil {
		t.Fatal(e)
	}
	base.URL = "udp://10.77.0.3:12000"
	if _, e := s.Save(base, 1); e == nil {
		t.Fatal("unicast address not assigned to receiver accepted")
	}
}

func TestUDPDatagramsRemainWholeAcrossSmallReadsAndCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	conn, e := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if e != nil {
		t.Fatal(e)
	}
	reader := contextual(ctx, &datagrams{conn: conn, buffer: make([]byte, 65535)})
	defer reader.Close()
	sender, e := net.DialUDP("udp4", nil, conn.LocalAddr().(*net.UDPAddr))
	if e != nil {
		t.Fatal(e)
	}
	defer sender.Close()
	data := bytes.Repeat(packet(), 7)
	if _, e = sender.Write(data); e != nil {
		t.Fatal(e)
	}
	result := make([]byte, len(data))
	for i := 0; i < 7; i++ {
		if _, e = io.ReadFull(reader, result[i*188:(i+1)*188]); e != nil {
			t.Fatal(e)
		}
	}
	if !bytes.Equal(result, data) {
		t.Fatal("UDP datagram truncated")
	}
	cancel()
	done := make(chan struct{})
	go func() { _, _ = reader.Read(result); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("UDP read not canceled")
	}
}

func TestHLSReferencesAreBoundedOpaqueSameOriginAndExtensionAware(t *testing.T) {
	u, _ := parse("https://user:synthetic-password@media.example/channel/index.m3u8?token=synthetic-token")
	refs := &references{root: u, base: "http://127.0.0.1:12345/private/", items: map[string]string{}}
	data, e := refs.rewrite([]byte("#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=\"key.bin\"\n#EXTINF:2,\nsegment.ts\n"), u)
	if e != nil || bytes.Contains(data, []byte("synthetic-")) || !bytes.Contains(data, []byte(".ts")) {
		t.Fatal("private rewrite", e)
	}
	for _, bad := range []string{"#EXTM3U\nhttps://external.example/segment.ts\n", "#EXTM3U\nfile:///etc/passwd\n", "#EXTM3U\n#EXT-X-KEY:METHOD=AES-128,URI=https://external.example/key\n"} {
		if _, e = refs.rewrite([]byte(bad), u); e == nil {
			t.Fatal("unsafe HLS reference accepted")
		}
	}
	for i := 0; i < 1100; i++ {
		if _, e = refs.add("segment.ts", u); e != nil {
			t.Fatal(e)
		}
	}
	if len(refs.items) != 1024 {
		t.Fatal("unbounded HLS reference set")
	}
	playlist, e := refs.add("variant.m3u8", u)
	if e != nil {
		t.Fatal(e)
	}
	key := strings.TrimPrefix(playlist, refs.base)
	for i := 0; i < 1200; i++ {
		if i%50 == 0 && refs.get(key) == "" {
			t.Fatal("active media playlist was evicted")
		}
		refs.add("segment.ts", u)
	}
	if refs.get(key) == "" || len(refs.items) > 1024 {
		t.Fatal("HLS reload reference lost or cache unbounded")
	}
}

type deniedCatalog struct{}

func (deniedCatalog) Snapshot() edgecatalog.Snapshot {
	return edgecatalog.Snapshot{Available: false, Catalog: &edgecatalog.Manifest{Channels: []edgecatalog.Channel{{ID: "remote", Protocols: []edgecatalog.Protocol{{ID: "http_ts"}}, Outputs: []string{"udp"}}}}}
}
func (deniedCatalog) Authorized(string, string) bool                          { return false }
func (deniedCatalog) OutputAuthorized(string, string) bool                    { return false }
func (deniedCatalog) OpenHTTP(context.Context, string) (io.ReadCloser, error) { return nil, ErrDenied }
func (deniedCatalog) OpenHLS(context.Context, string, string) (io.ReadCloser, error) {
	return nil, ErrDenied
}
func (deniedCatalog) OpenSRT(context.Context, string) (io.ReadCloser, error) { return nil, ErrDenied }
func TestLocalSourcesNeverExtendAnExpiredPartnerGrant(t *testing.T) {
	s := fixture(t)
	s.fallback = deniedCatalog{}
	id, e := s.Save(Source{URL: "https://media.example/a.ts", Enabled: true}, 0)
	if e != nil {
		t.Fatal(e)
	}
	snap := s.Snapshot()
	if !snap.Available || len(snap.Catalog.Channels) != 2 || len(snap.Catalog.Channels[0].Protocols) != 0 {
		t.Fatal("combined inventory")
	}
	if !s.Authorized(id, "http_ts") || s.Authorized("remote", "http_ts") || s.OutputAuthorized("remote", "udp") {
		t.Fatal("partner revocation bypassed")
	}
}
