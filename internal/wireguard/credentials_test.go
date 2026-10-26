package wireguard

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"golang.org/x/crypto/curve25519"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func syntheticSettings(t *testing.T) PeerSettings {
	t.Helper()
	key, e := ecdh.X25519().GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return PeerSettings{Interface: "wg-edge", Address: "10.77.0.2/32", ListenPort: 51821, KorePlayPublicKey: base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()), KorePlayEndpoint: "matrix.example:51820", AllowedIPs: []string{"10.77.0.1/32"}, Keepalive: 25}
}
func TestCredentialIdentityPersistenceAndPublicExport(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "wg")
	s, e := OpenCredentials(directory)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	initial, _ := s.Snapshot()
	if initial.State != "unconfigured" || initial.PublicKey != "" {
		t.Fatal("not clean")
	}
	if _, e := OpenCredentials(directory); e == nil {
		t.Fatal("concurrent writer accepted")
	}
	generated, e := s.EnsureIdentity()
	if e != nil {
		t.Fatal(e)
	}
	var file credentialFile
	bytes, e := os.ReadFile(filepath.Join(directory, "identity.json"))
	if e != nil || json.Unmarshal(bytes, &file) != nil {
		t.Fatal("identity persistence")
	}
	secret, e := base64.StdEncoding.DecodeString(file.PrivateKey)
	if e != nil || len(secret) != 32 || secret[0]&7 != 0 || secret[31]&128 != 0 || secret[31]&64 == 0 {
		t.Fatal("invalid WireGuard scalar")
	}
	public, e := curve25519.X25519(secret, curve25519.Basepoint)
	if e != nil || base64.StdEncoding.EncodeToString(public) != generated.PublicKey {
		t.Fatal("incompatible public key")
	}
	if tool := os.Getenv("KORE_EDGE_WG"); tool != "" {
		command := exec.Command(tool, "pubkey")
		command.Stdin = strings.NewReader(file.PrivateKey + "\n")
		output, e := command.Output()
		if e != nil || strings.TrimSpace(string(output)) != generated.PublicKey {
			t.Fatal("official wg pubkey compatibility failed")
		}
	}
	settings := syntheticSettings(t)
	configured, e := s.SavePeer(settings, generated.Revision)
	if e != nil {
		t.Fatal(e)
	}
	if configured.Applied || configured.State != "peer_prepared" || !strings.Contains(configured.KorePlayPeer, "AllowedIPs = 10.77.0.2/32") || strings.Contains(configured.KorePlayPeer, settings.KorePlayPublicKey) {
		t.Fatal("incorrect peer direction")
	}
	encoded, _ := json.Marshal(configured)
	if strings.Contains(string(encoded), file.PrivateKey) || strings.Contains(string(encoded), "private_key") || strings.Contains(string(encoded), "PrivateKey") {
		t.Fatal("secret exposed")
	}
	configured.Settings.AllowedIPs[0] = "tampered"
	settings.AllowedIPs[0] = "tampered"
	current, _ := s.Snapshot()
	if current.Settings.AllowedIPs[0] != "10.77.0.1/32" {
		t.Fatal("aliased settings")
	}
	if _, e = s.SavePeer(syntheticSettings(t), 1); e != ErrRevision {
		t.Fatal("stale update accepted")
	}
	if e = s.Close(); e != nil {
		t.Fatal(e)
	}
	s, e = OpenCredentials(directory)
	if e != nil {
		t.Fatal(e)
	}
	defer s.Close()
	reloaded, e := s.EnsureIdentity()
	if e != nil || reloaded.PublicKey != generated.PublicKey || reloaded.Revision != 2 {
		t.Fatal("identity changed on reopen")
	}
}
func TestPeerSettingsValidation(t *testing.T) {
	baseline := syntheticSettings(t)
	if baseline.Validate() != nil {
		t.Fatal("valid peer rejected")
	}
	for _, modify := range []func(*PeerSettings){
		func(s *PeerSettings) { s.Interface = "wg;reboot" }, func(s *PeerSettings) { s.Address = "10.77.0.2/24" }, func(s *PeerSettings) { s.ListenPort = 0 }, func(s *PeerSettings) { s.KorePlayPublicKey = base64.StdEncoding.EncodeToString(make([]byte, 32)) },
		func(s *PeerSettings) { s.KorePlayEndpoint = "matrix.example:51820\nPostUp=reboot" }, func(s *PeerSettings) { s.EdgeEndpoint = "0.0.0.0:10" }, func(s *PeerSettings) { s.KorePlayEndpoint = "matrix.example:0" },
		func(s *PeerSettings) { s.AllowedIPs = []string{"0.0.0.0/0"} }, func(s *PeerSettings) { s.AllowedIPs = []string{"10.77.0.0/24"} }, func(s *PeerSettings) { s.AllowedIPs = []string{"10.78.0.0/24", "10.78.0.1/32"} }, func(s *PeerSettings) { s.AllowedIPs = []string{"224.0.0.0/4"} }, func(s *PeerSettings) { s.AllowedIPs = []string{"10.77.0.3/24"} }, func(s *PeerSettings) { s.Keepalive = -1 },
	} {
		next := baseline
		modify(&next)
		if next.Validate() == nil {
			t.Fatal("invalid settings accepted")
		}
	}
	for _, endpoint := range []string{"203.0.113.8:51820", "192.168.10.1:51820", "[2001:db8::1]:51820", "matrix.example:51820"} {
		if !validEndpoint(endpoint) {
			t.Fatal("valid endpoint rejected", endpoint)
		}
	}
}
func TestIdentityConcurrentGenerationAndCorruption(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "wg")
	s, e := OpenCredentials(directory)
	if e != nil {
		t.Fatal(e)
	}
	var group sync.WaitGroup
	keys := make(chan string, 8)
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			v, e := s.EnsureIdentity()
			if e != nil {
				t.Error(e)
				return
			}
			keys <- v.PublicKey
		}()
	}
	group.Wait()
	close(keys)
	key := ""
	for value := range keys {
		if key != "" && key != value {
			t.Fatal("concurrent rotation")
		}
		key = value
	}
	s.Close()
	path := filepath.Join(directory, "identity.json")
	if os.WriteFile(path, []byte(`{"version":1,"private_key":"invalid"}`), 0600) != nil {
		t.Fatal("fixture")
	}
	if replacement, e := OpenCredentials(directory); e == nil {
		replacement.Close()
		t.Fatal("corrupt identity silently replaced")
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "invalid") {
		t.Fatal("corruption overwritten")
	}
}

func TestCredentialsRejectPublicPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission test")
	}
	directory := filepath.Join(t.TempDir(), "wg")
	s, e := OpenCredentials(directory)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.EnsureIdentity(); e != nil {
		s.Close()
		t.Fatal(e)
	}
	s.Close()
	path := filepath.Join(directory, "identity.json")
	info, e := os.Stat(path)
	if e != nil || info.Mode().Perm() != 0600 {
		t.Fatal("identity is not private")
	}
	if e = os.Chmod(path, 0644); e != nil {
		t.Fatal(e)
	}
	if reopened, e := OpenCredentials(directory); e == nil {
		reopened.Close()
		t.Fatal("publicly readable private key accepted")
	}
}
