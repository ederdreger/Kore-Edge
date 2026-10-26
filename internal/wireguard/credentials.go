package wireguard

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/security"
)

var ErrCredentials = errors.New("wireguard credentials unavailable")
var ErrPeerSettings = errors.New("invalid wireguard peer settings")
var ErrRevision = errors.New("wireguard settings changed; reload before saving")

// PeerSettings is an operator-confirmed proposal, never applied to the host.
type PeerSettings struct {
	Interface         string   `json:"interface"`
	Address           string   `json:"address"`
	ListenPort        int      `json:"listen_port"`
	EdgeEndpoint      string   `json:"edge_endpoint"`
	KorePlayPublicKey string   `json:"kore_play_public_key"`
	KorePlayEndpoint  string   `json:"kore_play_endpoint"`
	AllowedIPs        []string `json:"allowed_ips"`
	Keepalive         int      `json:"keepalive_seconds"`
}

func validEndpoint(raw string) bool {
	host, port, e := net.SplitHostPort(raw)
	if e != nil || len(raw) > 300 {
		return false
	}
	n, e := strconv.Atoi(port)
	if e != nil || n < 1 || n > 65535 || strconv.Itoa(n) != port {
		return false
	}
	if ip, e := netip.ParseAddr(host); e == nil {
		return ip.IsGlobalUnicast() && !ip.IsLoopback() && !ip.Is4In6() && ip.Zone() == ""
	}
	if len(host) > 253 || !strings.Contains(host, ".") || strings.Trim(host, "0123456789.") == "" {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}

func validCurveKey(raw string) bool {
	if !validKey(raw) {
		return false
	}
	b, _ := base64.StdEncoding.DecodeString(raw)
	p, e := ecdh.X25519().NewPublicKey(b)
	if e != nil {
		return false
	}
	// A non-secret probe rejects low-order public points rejected by X25519.
	probe := make([]byte, 32)
	probe[0] = 9
	k, e := ecdh.X25519().NewPrivateKey(probe)
	if e != nil {
		return false
	}
	_, e = k.ECDH(p)
	return e == nil
}

func (s PeerSettings) Validate() error {
	a, e := netip.ParsePrefix(s.Address)
	if !namePattern.MatchString(s.Interface) || e != nil || !privatePrefix(a) || a.Addr().Is4In6() || a.Bits() != a.Addr().BitLen() || s.Address != a.String() || s.ListenPort < 1 || s.ListenPort > 65535 {
		return ErrPeerSettings
	}
	if !validCurveKey(s.KorePlayPublicKey) || !validEndpoint(s.KorePlayEndpoint) || (s.EdgeEndpoint != "" && !validEndpoint(s.EdgeEndpoint)) || s.Keepalive < 0 || s.Keepalive > 65535 || len(s.AllowedIPs) == 0 || len(s.AllowedIPs) > 32 {
		return ErrPeerSettings
	}
	var routes []netip.Prefix
	for _, raw := range s.AllowedIPs {
		p, e := netip.ParsePrefix(raw)
		if e != nil || p != p.Masked() || !privatePrefix(p) || p.Addr().Is4In6() || p.Contains(a.Addr()) {
			return ErrPeerSettings
		}
		for _, other := range routes {
			if p.Overlaps(other) {
				return ErrPeerSettings
			}
		}
		routes = append(routes, p)
	}
	return nil
}

// CredentialStatus deliberately has no field for secret key material.
type CredentialStatus struct {
	State        string        `json:"state"`
	Applied      bool          `json:"applied"`
	PublicKey    string        `json:"public_key"`
	Revision     uint64        `json:"revision"`
	CreatedAt    *time.Time    `json:"created_at"`
	Settings     *PeerSettings `json:"settings"`
	KorePlayPeer string        `json:"kore_play_peer"`
}
type credentialFile struct {
	Version    int           `json:"version"`
	PrivateKey string        `json:"private_key"`
	CreatedAt  time.Time     `json:"created_at"`
	Revision   uint64        `json:"revision"`
	Settings   *PeerSettings `json:"settings"`
}
type CredentialStore struct {
	mu                   sync.Mutex
	directory            string
	lock                 *os.File
	data                 credentialFile
	closed               bool
	distributionRevision uint64
	distributionPublic   string
}

func OpenCredentials(directory string) (*CredentialStore, error) {
	if security.PrivateDirectory(directory, true) != nil {
		return nil, ErrCredentials
	}
	lock, e := credentialLock(filepath.Join(directory, "credentials.lock"))
	if e != nil {
		return nil, ErrCredentials
	}
	s := &CredentialStore{directory: directory, lock: lock}
	if _, e = os.Lstat(filepath.Join(directory, "identity.json")); !os.IsNotExist(e) {
		b, e := security.ReadPrivateFile(filepath.Join(directory, "identity.json"), 32*1024)
		if e != nil || catalog.DecodeStrict(b, &s.data) != nil || s.validate() != nil {
			s.Close()
			return nil, ErrCredentials
		}
	}
	return s, nil
}
func (s *CredentialStore) validate() error {
	b, e := base64.StdEncoding.DecodeString(s.data.PrivateKey)
	if e != nil || len(b) != 32 || base64.StdEncoding.EncodeToString(b) != s.data.PrivateKey || b[0]&7 != 0 || b[31]&128 != 0 || b[31]&64 == 0 || s.data.Version != 1 || s.data.Revision == 0 || s.data.CreatedAt.IsZero() {
		return ErrCredentials
	}
	if s.publicKey() == "" {
		return ErrCredentials
	}
	if s.data.Settings != nil && (s.data.Settings.Validate() != nil || s.data.Settings.KorePlayPublicKey == s.publicKey()) {
		return ErrCredentials
	}
	return nil
}
func (s *CredentialStore) publicKey() string {
	b, e := base64.StdEncoding.DecodeString(s.data.PrivateKey)
	if e != nil || len(b) != 32 {
		return ""
	}
	key, e := ecdh.X25519().NewPrivateKey(b)
	if e != nil {
		return ""
	}
	return base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
}
func (s *CredentialStore) snapshot() CredentialStatus {
	out := CredentialStatus{State: "unconfigured"}
	if s.data.PrivateKey == "" {
		return out
	}
	t := s.data.CreatedAt
	out.CreatedAt = &t
	out.PublicKey = s.publicKey()
	out.Revision = s.data.Revision
	out.State = "credentials_generated"
	if s.data.Settings != nil {
		settings := *s.data.Settings
		settings.AllowedIPs = append([]string(nil), settings.AllowedIPs...)
		out.Settings = &settings
		out.State = "peer_prepared"
		out.KorePlayPeer = fmt.Sprintf("[Peer]\nPublicKey = %s\nAllowedIPs = %s\n", out.PublicKey, settings.Address)
		if settings.EdgeEndpoint != "" {
			out.KorePlayPeer += "Endpoint = " + settings.EdgeEndpoint + "\n"
		}
	}
	return out
}
func (s *CredentialStore) Snapshot() (CredentialStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return CredentialStatus{}, ErrCredentials
	}
	return s.snapshot(), nil
}
func (s *CredentialStore) persist(next credentialFile) error {
	b, e := json.Marshal(next)
	if e != nil {
		return ErrCredentials
	}
	f, e := os.CreateTemp(s.directory, ".identity-")
	if e != nil {
		return ErrCredentials
	}
	defer os.Remove(f.Name())
	if f.Chmod(0600) != nil {
		f.Close()
		return ErrCredentials
	}
	if n, e := f.Write(b); e != nil || n != len(b) {
		f.Close()
		return ErrCredentials
	}
	if f.Sync() != nil {
		f.Close()
		return ErrCredentials
	}
	if f.Close() != nil || os.Rename(f.Name(), filepath.Join(s.directory, "identity.json")) != nil {
		return ErrCredentials
	}
	s.data = next
	return credentialSync(s.directory)
}

// EnsureIdentity is idempotent; retries and upgrades never replace an identity.
func (s *CredentialStore) EnsureIdentity() (CredentialStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return CredentialStatus{}, ErrCredentials
	}
	if s.data.PrivateKey == "" {
		var b [32]byte
		if _, e := rand.Read(b[:]); e != nil {
			return CredentialStatus{}, ErrCredentials
		}
		b[0] &= 248
		b[31] &= 127
		b[31] |= 64
		if _, e := ecdh.X25519().NewPrivateKey(b[:]); e != nil {
			return CredentialStatus{}, ErrCredentials
		}
		next := credentialFile{Version: 1, PrivateKey: base64.StdEncoding.EncodeToString(b[:]), Revision: 1, CreatedAt: time.Now().UTC()}
		if e := s.persist(next); e != nil {
			return CredentialStatus{}, e
		}
	}
	return s.snapshot(), nil
}
func (s *CredentialStore) SavePeer(settings PeerSettings, expected uint64) (CredentialStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.data.PrivateKey == "" {
		return CredentialStatus{}, ErrCredentials
	}
	if expected != s.data.Revision {
		return CredentialStatus{}, ErrRevision
	}
	if settings.Validate() != nil || settings.KorePlayPublicKey == s.publicKey() {
		return CredentialStatus{}, ErrPeerSettings
	}
	settings.AllowedIPs = append([]string(nil), settings.AllowedIPs...)
	next := s.data
	next.Settings = &settings
	next.Revision++
	if next.Revision == 0 {
		return CredentialStatus{}, ErrCredentials
	}
	if e := s.persist(next); e != nil {
		return CredentialStatus{}, e
	}
	return s.snapshot(), nil
}
func (s *CredentialStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	return s.lock.Close()
}
