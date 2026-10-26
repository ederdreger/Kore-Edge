// Package edgecatalog receives partner-authorized real catalogs without changing
// the synthetic catalog/stream contracts from the laboratory modules.
package edgecatalog

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"kore-edge/internal/catalog"
	"kore-edge/internal/security"
	"kore-edge/internal/wireguard"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

var ErrUnavailable = errors.New("catalog_unavailable")
var ErrInvalid = errors.New("invalid_catalog_configuration")
var ErrConflict = errors.New("catalog_configuration_changed")
var ErrIntegrity = errors.New("invalid_distribution_reply")
var ErrDenied = errors.New("partner_not_authorized")
var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)

type Config struct {
	Origin   string `json:"origin"`
	TunnelIP string `json:"tunnel_ip"`
	AutoSync bool   `json:"auto_sync"`
}
type Protocol struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Path         string `json:"path"`
	Verification string `json:"verification"`
}
type Channel struct {
	Outputs   []string   `json:"outputs,omitempty"`
	ID        string     `json:"id"`
	Name      string     `json:"name"`
	Category  string     `json:"category"`
	Protocols []Protocol `json:"protocols"`
}
type Manifest struct {
	SRT         *SRTTransport `json:"srt,omitempty"`
	Version     int           `json:"version"`
	Revision    uint64        `json:"grant_revision"`
	PartnerID   string        `json:"partner_id"`
	PartnerName string        `json:"partner_name"`
	IssuedAt    time.Time     `json:"issued_at"`
	ExpiresAt   time.Time     `json:"expires_at"`
	Channels    []Channel     `json:"channels"`
}
type state struct {
	Version    int       `json:"version"`
	Generation uint64    `json:"generation"`
	Config     Config    `json:"config"`
	EdgeKey    string    `json:"edge_key"`
	MatrixKey  string    `json:"matrix_key"`
	Catalog    *Manifest `json:"catalog"`
	LastSync   time.Time `json:"last_sync"`
	Error      string    `json:"error"`
}
type Snapshot struct {
	Generation uint64    `json:"generation"`
	Config     Config    `json:"config"`
	Configured bool      `json:"configured"`
	Available  bool      `json:"available"`
	Catalog    *Manifest `json:"catalog"`
	LastSync   time.Time `json:"last_sync"`
	Error      string    `json:"error"`
}
type Service struct {
	mu          sync.Mutex
	work        sync.Mutex
	data        state
	directory   string
	credentials *wireguard.CredentialStore
	client      func(Config, *wireguard.PeerSettings) *http.Client
	mediaSlots  chan struct{}
	probeSlots  chan struct{}
}

func New(directory string, credentials *wireguard.CredentialStore) (*Service, error) {
	if credentials == nil || security.PrivateDirectory(directory, true) != nil {
		return nil, ErrUnavailable
	}
	s := &Service{directory: directory, credentials: credentials, client: secureClient, mediaSlots: make(chan struct{}, 64), probeSlots: make(chan struct{}, 2), data: state{Version: 1}}
	path := filepath.Join(directory, "catalog.json")
	if _, e := os.Lstat(path); !os.IsNotExist(e) {
		b, e := security.ReadPrivateFile(path, 4*1024*1024)
		if e != nil || catalog.DecodeStrict(b, &s.data) != nil || s.data.Version != 1 {
			return nil, ErrUnavailable
		}
		if s.data.Catalog != nil && s.data.Catalog.validate(s.data.Catalog.IssuedAt) != nil {
			return nil, ErrUnavailable
		}
	}
	return s, nil
}
func (c Config) validate(settings *wireguard.PeerSettings) error {
	u, e := url.Parse(c.Origin)
	if e != nil || len(c.Origin) > 2048 || u.Scheme != "https" || u.User != nil || u.Hostname() == "" || u.Path != "" || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.Opaque != "" || settings == nil {
		return ErrInvalid
	}
	port := u.Port()
	if port == "" {
		port = "443"
	}
	if number, e := strconv.Atoi(port); e != nil || number < 1 || number > 65535 {
		return ErrInvalid
	}
	ip, e := netip.ParseAddr(c.TunnelIP)
	if e != nil || !ip.IsPrivate() || ip.Is4In6() || ip.Zone() != "" || ip.String() != c.TunnelIP {
		return ErrInvalid
	}
	local, e := netip.ParsePrefix(settings.Address)
	if e != nil || local.Addr() == ip || local.Addr().Is4() != ip.Is4() {
		return ErrInvalid
	}
	allowed := false
	for _, raw := range settings.AllowedIPs {
		p, e := netip.ParsePrefix(raw)
		if e == nil && p.Contains(ip) {
			allowed = true
		}
	}
	if !allowed {
		return ErrInvalid
	}
	return nil
}
func secureClient(c Config, settings *wireguard.PeerSettings) *http.Client {
	origin, _ := url.Parse(c.Origin)
	port := origin.Port()
	if port == "" {
		port = "443"
	}
	local, _ := netip.ParsePrefix(settings.Address)
	transport := &http.Transport{Proxy: nil, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: 2 * time.Second, ResponseHeaderTimeout: 3 * time.Second, MaxIdleConns: 16, MaxIdleConnsPerHost: 8, IdleConnTimeout: 30 * time.Second, DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: time.Second, LocalAddr: &net.TCPAddr{IP: net.ParseIP(local.Addr().String())}}).DialContext(ctx, network, net.JoinHostPort(c.TunnelIP, port))
	}}
	return &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return ErrDenied }}
}
func (s *Service) save() error {
	b, e := json.Marshal(s.data)
	if e != nil {
		return ErrUnavailable
	}
	f, e := os.CreateTemp(s.directory, ".catalog-")
	if e != nil {
		return ErrUnavailable
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return ErrUnavailable
	}
	if f.Sync() != nil {
		f.Close()
		return ErrUnavailable
	}
	if f.Close() != nil || os.Rename(f.Name(), filepath.Join(s.directory, "catalog.json")) != nil {
		return ErrUnavailable
	}
	if os.PathSeparator != '\\' {
		d, e := os.Open(s.directory)
		if e != nil {
			return ErrUnavailable
		}
		defer d.Close()
		if d.Sync() != nil {
			return ErrUnavailable
		}
	}
	return nil
}
func (s *Service) Configure(c Config, expected uint64) error {
	if !s.work.TryLock() {
		return ErrConflict
	}
	defer s.work.Unlock()
	identity, e := s.credentials.Snapshot()
	if e != nil || c.validate(identity.Settings) != nil {
		return ErrInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected != s.data.Generation {
		return ErrConflict
	}
	old := s.data
	s.data = state{Version: 1, Generation: old.Generation + 1, Config: c, EdgeKey: identity.PublicKey, MatrixKey: identity.Settings.KorePlayPublicKey}
	if e = s.save(); e != nil {
		s.data = old
		return e
	}
	return nil
}
func (s *Service) Snapshot() Snapshot {
	s.mu.Lock()
	b, _ := json.Marshal(s.data)
	s.mu.Unlock()
	var data state
	_ = json.Unmarshal(b, &data)
	available := data.Catalog != nil && time.Now().Before(data.Catalog.ExpiresAt)
	identity, e := s.credentials.Snapshot()
	if e != nil || identity.Settings == nil || identity.PublicKey != data.EdgeKey || identity.Settings.KorePlayPublicKey != data.MatrixKey {
		available = false
		if data.Config.Origin != "" {
			data.Error = "identity_changed"
		}
	}
	return Snapshot{Generation: data.Generation, Config: data.Config, Configured: data.Config.Origin != "", Available: available, Catalog: data.Catalog, LastSync: data.LastSync, Error: data.Error}
}
func (s *Service) remote(ctx context.Context, path string, ranges ...string) (*http.Response, string, error) {
	snap := s.Snapshot()
	identity, e := s.credentials.Snapshot()
	if e != nil || !snap.Configured || snap.Config.validate(identity.Settings) != nil || snap.Error == "identity_changed" {
		return nil, "", ErrInvalid
	}
	headers, e := s.credentials.DistributionHeaders(path)
	if e != nil {
		return nil, "", ErrUnavailable
	}
	request, e := http.NewRequestWithContext(ctx, "GET", snap.Config.Origin+path, nil)
	if e != nil {
		return nil, "", ErrInvalid
	}
	request.Header = headers
	if len(ranges) > 0 {
		request.Header.Set("Range", ranges[0])
	}
	client := s.client(snap.Config, identity.Settings)
	response, e := client.Do(request)
	if e != nil {
		client.CloseIdleConnections()
		return nil, "", ErrUnavailable
	}
	// Each request owns its transport. Closing response releases the socket; no
	// unbounded pool is retained across configuration changes or partner requests.
	response.Body = &closingBody{ReadCloser: response.Body, close: client.CloseIdleConnections}
	if response.StatusCode != 200 && response.StatusCode != 206 {
		response.Body.Close()
		if response.StatusCode == 401 || response.StatusCode == 403 {
			return nil, "", ErrDenied
		}
		return nil, "", ErrUnavailable
	}
	return response, headers.Get("X-Kore-Edge-Nonce"), nil
}

type closingBody struct {
	io.ReadCloser
	close func()
}

func (b *closingBody) Close() error { e := b.ReadCloser.Close(); b.close(); return e }
func (s *Service) signed(ctx context.Context, path string, target any) error {
	response, nonce, e := s.remote(ctx, path)
	if e != nil {
		return e
	}
	defer response.Body.Close()
	body, e := io.ReadAll(io.LimitReader(response.Body, 4*1024*1024+1))
	if e != nil || len(body) > 4*1024*1024 {
		return ErrUnavailable
	}
	var envelope struct {
		Payload string `json:"payload"`
		Proof   string `json:"proof"`
	}
	if catalog.DecodeStrict(body, &envelope) != nil || !s.credentials.VerifyDistribution(nonce, envelope.Payload, envelope.Proof) {
		return ErrIntegrity
	}
	decoded, e := base64.StdEncoding.DecodeString(envelope.Payload)
	if e != nil || len(decoded) > 2*1024*1024 || catalog.DecodeStrict(decoded, target) != nil {
		return ErrIntegrity
	}
	return nil
}
func (m Manifest) validate(now time.Time) error {
	if m.SRT != nil && (m.SRT.Port < 1024 || m.SRT.Port > 65535 || m.SRT.LatencyMS < 20 || m.SRT.LatencyMS > 5000) {
		return ErrIntegrity
	}
	if m.Version != 1 || m.Revision < 1 || !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(m.PartnerID) || len(m.PartnerName) < 1 || len(m.PartnerName) > 512 || m.IssuedAt.After(now.Add(30*time.Second)) || !m.ExpiresAt.After(now) || m.ExpiresAt.Sub(m.IssuedAt) > 5*time.Minute || !m.ExpiresAt.After(m.IssuedAt) || len(m.Channels) > 512 {
		return ErrIntegrity
	}
	seen := map[string]bool{}
	for _, c := range m.Channels {
		if len(c.Outputs) > 2 {
			return ErrIntegrity
		}
		seenOutputs := map[string]bool{}
		for _, output := range c.Outputs {
			if (output != "udp" && output != "hls") || seenOutputs[output] {
				return ErrIntegrity
			}
			seenOutputs[output] = true
		}
		if !safeID.MatchString(c.ID) || seen[c.ID] || len(c.Name) < 1 || len(c.Name) > 512 || len(c.Category) > 512 || len(c.Protocols) > 2 {
			return ErrIntegrity
		}
		seen[c.ID] = true
		protocols := map[string]bool{}
		for _, p := range c.Protocols {
			suffix := "/stream.ts"
			if p.ID == "hls" {
				suffix = "/index.m3u8"
			} else if p.ID != "http_ts" && p.ID != "srt" {
				return ErrIntegrity
			}
			if p.ID == "srt" && m.SRT == nil {
				return ErrIntegrity
			}
			if protocols[p.ID] || p.Path != "/api/kore-edge/media/"+c.ID+"/"+p.ID+suffix || p.Verification != "not_tested" {
				return ErrIntegrity
			}
			protocols[p.ID] = true
		}
	}
	return nil
}
func (s *Service) Sync(ctx context.Context) error {
	if !s.work.TryLock() {
		return ErrConflict
	}
	defer s.work.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	var manifest Manifest
	e := s.signed(ctx, "/api/kore-edge/catalog", &manifest)
	if e == nil {
		e = manifest.validate(time.Now())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	old := s.data
	if e == nil && old.Catalog != nil && (old.Catalog.PartnerID != manifest.PartnerID || manifest.Revision < old.Catalog.Revision) {
		e = ErrIntegrity
	}
	if e != nil {
		s.data.Error = "sync_failed"
		if errors.Is(e, ErrDenied) {
			s.data.Catalog = nil
			s.data.Error = "authorization_denied"
		}
		_ = s.save()
		return e
	}
	s.data.Catalog = &manifest
	s.data.LastSync = time.Now().UTC()
	s.data.Error = ""
	if e = s.save(); e != nil {
		s.data = old
		return e
	}
	return nil
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		if snap := s.Snapshot(); snap.Configured && snap.Config.AutoSync {
			_ = s.Sync(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) allowed(id, protocol string) bool {
	identity, e := s.credentials.Snapshot()
	if e != nil || identity.Settings == nil {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data := s.data
	if data.Catalog == nil || !time.Now().Before(data.Catalog.ExpiresAt) || identity.PublicKey != data.EdgeKey || identity.Settings.KorePlayPublicKey != data.MatrixKey {
		return false
	}
	for _, channel := range data.Catalog.Channels {
		if channel.ID == id {
			for _, p := range channel.Protocols {
				if p.ID == protocol {
					return true
				}
			}
		}
	}
	return false
}
func mediaPath(id, protocol, tail string) (string, error) {
	if !safeID.MatchString(id) {
		return "", ErrInvalid
	}
	if protocol == "http_ts" && tail == "stream.ts" || protocol == "hls" && (tail == "index.m3u8" || strings.HasPrefix(tail, "r/") && len(tail) >= 42 && len(tail) <= 8194 && regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(strings.TrimPrefix(tail, "r/"))) {
		return "/api/kore-edge/media/" + id + "/" + protocol + "/" + tail, nil
	}
	return "", ErrInvalid
}
