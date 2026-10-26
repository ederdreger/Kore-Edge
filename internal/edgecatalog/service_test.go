package edgecatalog

import (
	"context"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"kore-edge/internal/wireguard"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func proof(key []byte, text string) string {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write([]byte(text))
	return hex.EncodeToString(m.Sum(nil))
}
func fixture(t *testing.T) (*Service, *wireguard.CredentialStore, []byte) {
	t.Helper()
	credentials, e := wireguard.OpenCredentials(filepath.Join(t.TempDir(), "identity"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { credentials.Close() })
	identity, e := credentials.EnsureIdentity()
	if e != nil {
		t.Fatal(e)
	}
	matrix, _ := ecdh.X25519().GenerateKey(rand.Reader)
	raw, _ := base64.StdEncoding.DecodeString(identity.PublicKey)
	remote, _ := ecdh.X25519().NewPublicKey(raw)
	secret, _ := matrix.ECDH(remote)
	_, e = credentials.SavePeer(wireguard.PeerSettings{Interface: "wg-test", Address: "10.77.0.2/32", ListenPort: 51821, KorePlayPublicKey: base64.StdEncoding.EncodeToString(matrix.PublicKey().Bytes()), KorePlayEndpoint: "matrix.example:51820", AllowedIPs: []string{"10.77.0.1/32"}, Keepalive: 25}, identity.Revision)
	if e != nil {
		t.Fatal(e)
	}
	service, e := New(filepath.Join(t.TempDir(), "catalog"), credentials)
	if e != nil {
		t.Fatal(e)
	}
	if e = service.Configure(Config{Origin: "https://matrix.example:443", TunnelIP: "10.77.0.1"}, 0); e != nil {
		t.Fatal(e)
	}
	return service, credentials, secret
}
func manifest() Manifest {
	now := time.Now().UTC()
	return Manifest{Version: 1, Revision: 1, PartnerID: strings.Repeat("a", 32), PartnerName: "Synthetic Partner", IssuedAt: now, ExpiresAt: now.Add(5 * time.Minute), Channels: []Channel{{ID: "channel-one", Name: "Synthetic Channel", Category: "Lab", Protocols: []Protocol{{ID: "http_ts", Label: "HTTP MPEG-TS", Path: "/api/kore-edge/media/channel-one/http_ts/stream.ts", Verification: "not_tested"}}}}}
}

func TestSignedSynchronizationPersistenceAndRevocation(t *testing.T) {
	s, credentials, secret := fixture(t)
	document := manifest()
	tamper, denied := false, false
	s.client = func(_ Config, _ *wireguard.PeerSettings) *http.Client {
		return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			expected := proof(secret, "kore-edge/distribution/request/v1\nGET\n"+r.URL.Path+"\n"+r.Header.Get("X-Kore-Edge-Time")+"\n"+r.Header.Get("X-Kore-Edge-Nonce"))
			if r.Header.Get("X-Kore-Edge-Proof") != expected {
				t.Fatal("invalid proof of Edge identity")
			}
			if denied {
				return &http.Response{StatusCode: 403, Body: io.NopCloser(strings.NewReader("{}"))}, nil
			}
			payload, _ := json.Marshal(document)
			encoded := base64.StdEncoding.EncodeToString(payload)
			signature := proof(secret, "kore-edge/distribution/reply/v1\n"+r.Header.Get("X-Kore-Edge-Nonce")+"\n"+encoded)
			if tamper {
				signature = strings.Repeat("0", 64)
			}
			body, _ := json.Marshal(map[string]string{"payload": encoded, "proof": signature})
			return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
		})}
	}
	if e := s.Sync(context.Background()); e != nil {
		t.Fatal(e)
	}
	if !s.Snapshot().Available || !s.allowed("channel-one", "http_ts") || s.allowed("other", "http_ts") || s.allowed("channel-one", "hls") {
		t.Fatal("grant isolation failed")
	}
	reopened, e := New(s.directory, credentials)
	if e != nil || !reopened.Snapshot().Available {
		t.Fatal("durable catalog unavailable", e)
	}
	tamper = true
	if e = s.Sync(context.Background()); e != ErrIntegrity {
		t.Fatal("forged response accepted", e)
	}
	if !s.Snapshot().Available {
		t.Fatal("failed response discarded a still-valid lease")
	}
	tamper = false
	document.Revision = 0
	if e = s.Sync(context.Background()); e == nil {
		t.Fatal("invalid revision accepted")
	}
	denied = true
	if e = s.Sync(context.Background()); e != ErrDenied {
		t.Fatal(e)
	}
	if s.Snapshot().Available || s.allowed("channel-one", "http_ts") {
		t.Fatal("revoked partner retained access")
	}
}
func TestConfigurationIsPinnedAndExpiredCatalogDenied(t *testing.T) {
	s, _, _ := fixture(t)
	identity, _ := s.credentials.Snapshot()
	for _, config := range []Config{{Origin: "http://matrix.example", TunnelIP: "10.77.0.1"}, {Origin: "https://user:pass@matrix.example", TunnelIP: "10.77.0.1"}, {Origin: "https://matrix.example", TunnelIP: "127.0.0.1"}, {Origin: "https://matrix.example", TunnelIP: "10.88.0.1"}, {Origin: "https://matrix.example/path", TunnelIP: "10.77.0.1"}} {
		if config.validate(identity.Settings) == nil {
			t.Fatal("unsafe config accepted")
		}
	}
	if s.Configure(s.Snapshot().Config, 0) != ErrConflict {
		t.Fatal("stale generation accepted")
	}
	s.mu.Lock()
	m := manifest()
	m.ExpiresAt = time.Now().Add(-time.Second)
	s.data.Catalog = &m
	s.mu.Unlock()
	if s.allowed("channel-one", "http_ts") || s.Snapshot().Available {
		t.Fatal("expired lease accepted")
	}
	for _, tail := range []string{"../../etc/passwd", "https://evil.example", "r/../x"} {
		if _, e := mediaPath("channel-one", "hls", tail); e == nil {
			t.Fatal("unsafe resource path accepted")
		}
	}
}
func TestMediaRelaysSyntheticBytesAndDeniesUnlistedChannel(t *testing.T) {
	s, _, _ := fixture(t)
	s.mu.Lock()
	m := manifest()
	s.data.Catalog = &m
	s.mu.Unlock()
	sample := strings.Repeat("synthetic transport bytes", 64)
	s.client = func(_ Config, _ *wireguard.PeerSettings) *http.Client {
		return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
			if r.URL.Path != "/api/kore-edge/media/channel-one/http_ts/stream.ts" {
				t.Fatal("unexpected origin path")
			}
			return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"video/mp2t"}}, Body: io.NopCloser(strings.NewReader(sample))}, nil
		})}
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/denied" {
			s.Media(w, r, "other", "http_ts", "stream.ts")
		} else {
			s.Media(w, r, "channel-one", "http_ts", "stream.ts")
		}
	}))
	defer server.Close()
	response, e := http.Get(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if string(body) != sample {
		t.Fatal("relay modified bytes")
	}
	response, e = http.Get(server.URL + "/denied")
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("ungranted channel accepted")
	}
}
