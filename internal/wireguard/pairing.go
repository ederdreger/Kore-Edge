package wireguard

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"kore-edge/internal/catalog"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"time"
)

type Invitation struct {
	Version         int    `json:"version"`
	APIOrigin       string `json:"api_origin"`
	Token           string `json:"token"`
	EdgePublicKey   string `json:"edge_public_key"`
	MatrixPublicKey string `json:"matrix_public_key"`
	Address         string `json:"edge_address"`
	Expires         int64  `json:"expires_at"`
}
type LocalConnection struct {
	Interface  string `json:"interface"`
	ListenPort int    `json:"listen_port"`
	Keepalive  int    `json:"keepalive_seconds"`
}

func sign(key []byte, text string) string {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))
}
func ParseInvitation(raw string) (Invitation, error) {
	var inv Invitation
	if len(raw) > 4096 || catalog.DecodeStrict([]byte(raw), &inv) != nil || inv.Version != 1 || len(inv.Token) != 43 || !validCurveKey(inv.EdgePublicKey) || !validCurveKey(inv.MatrixPublicKey) || inv.Expires <= time.Now().UnixMilli() || inv.Expires > time.Now().Add(11*time.Minute).UnixMilli() {
		return inv, ErrPeerSettings
	}
	for _, c := range inv.Token {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return inv, ErrPeerSettings
		}
	}
	origin, e := url.Parse(inv.APIOrigin)
	if e != nil || origin.Scheme != "https" || origin.User != nil || origin.Hostname() == "" || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return inv, ErrPeerSettings
	}
	port := origin.Port()
	if port == "" {
		port = "443"
	}
	if !validEndpoint(net.JoinHostPort(origin.Hostname(), port)) {
		return inv, ErrPeerSettings
	}
	p, e := netip.ParsePrefix(inv.Address)
	if e != nil || !privatePrefix(p) || p.Bits() != p.Addr().BitLen() || p.String() != inv.Address {
		return inv, ErrPeerSettings
	}
	return inv, nil
}
func pairingClient(origin string) *http.Client {
	u, _ := url.Parse(origin)
	host := u.Hostname()
	literal, literalError := netip.ParseAddr(host)
	transport := &http.Transport{Proxy: nil, DisableKeepAlives: true, TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12}, TLSHandshakeTimeout: time.Second, ResponseHeaderTimeout: 2 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		_, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, ErrPeerSettings
		}
		ips, e := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
		if e != nil {
			return nil, ErrCredentials
		}
		for _, ip := range ips {
			ip = ip.Unmap()
			if !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || (ip.IsPrivate() && literalError != nil) {
				return nil, ErrPeerSettings
			}
			if literalError == nil && literal.Unmap() != ip {
				return nil, ErrPeerSettings
			}
		}
		for _, ip := range ips {
			connection, e := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
			if e == nil {
				return connection, nil
			}
		}
		return nil, ErrCredentials
	}}
	return &http.Client{Transport: transport, Timeout: 3 * time.Second, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return ErrPeerSettings }}
}

// ClaimConnection synchronizes public settings only. No network interface is
// applied until the separate reviewed local bridge plan is confirmed.
func (s *CredentialStore) ClaimConnection(ctx context.Context, raw string, local LocalConnection, expected uint64) (CredentialStatus, error) {
	return s.claimConnection(ctx, raw, local, expected, nil)
}
func (s *CredentialStore) claimConnection(ctx context.Context, raw string, local LocalConnection, expected uint64, client *http.Client) (CredentialStatus, error) {
	inv, e := ParseInvitation(raw)
	if e != nil || !namePattern.MatchString(local.Interface) || local.ListenPort < 1 || local.ListenPort > 65535 || local.Keepalive < 0 || local.Keepalive > 65535 {
		return CredentialStatus{}, ErrPeerSettings
	}
	s.mu.Lock()
	if s.closed || s.data.Revision != expected || s.publicKey() != inv.EdgePublicKey {
		s.mu.Unlock()
		return CredentialStatus{}, ErrRevision
	}
	if s.data.Settings != nil && s.data.Settings.KorePlayPublicKey != inv.MatrixPublicKey {
		s.mu.Unlock()
		return CredentialStatus{}, ErrPeerSettings
	}
	private := s.data.PrivateKey
	s.mu.Unlock()
	scalar, _ := base64.StdEncoding.DecodeString(private)
	key, e := ecdh.X25519().NewPrivateKey(scalar)
	if e != nil {
		return CredentialStatus{}, ErrCredentials
	}
	remote, _ := base64.StdEncoding.DecodeString(inv.MatrixPublicKey)
	public, e := ecdh.X25519().NewPublicKey(remote)
	if e != nil {
		return CredentialStatus{}, ErrPeerSettings
	}
	shared, e := key.ECDH(public)
	if e != nil {
		return CredentialStatus{}, ErrPeerSettings
	}
	nonce := sign(shared, "kore-edge/connect/nonce/v1\n"+inv.Token+"\n"+inv.EdgePublicKey)
	requestBody := map[string]string{"token": inv.Token, "public_key": inv.EdgePublicKey, "nonce": nonce, "proof": sign(shared, "kore-edge/connect/v1\n"+inv.Token+"\n"+inv.EdgePublicKey+"\n"+nonce)}
	b, _ := json.Marshal(requestBody)
	request, e := http.NewRequestWithContext(ctx, "POST", inv.APIOrigin+"/api/wireguard/connect/claim", bytes.NewReader(b))
	if e != nil {
		return CredentialStatus{}, ErrPeerSettings
	}
	request.Header.Set("Content-Type", "application/json")
	if client == nil {
		client = pairingClient(inv.APIOrigin)
		if t, ok := client.Transport.(*http.Transport); ok {
			defer t.CloseIdleConnections()
		}
	}
	response, e := client.Do(request)
	if e != nil {
		return CredentialStatus{}, ErrCredentials
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return CredentialStatus{}, ErrCredentials
	}
	body, e := io.ReadAll(io.LimitReader(response.Body, 32769))
	if e != nil || len(body) > 32768 {
		return CredentialStatus{}, ErrCredentials
	}
	var envelope struct {
		Payload string `json:"payload"`
		Proof   string `json:"proof"`
	}
	if catalog.DecodeStrict(body, &envelope) != nil {
		return CredentialStatus{}, ErrCredentials
	}
	want := sign(shared, "kore-edge/connect/reply/v1\n"+nonce+"\n"+envelope.Payload)
	got, e := hex.DecodeString(envelope.Proof)
	wantBytes, _ := hex.DecodeString(want)
	if e != nil || !hmac.Equal(got, wantBytes) {
		return CredentialStatus{}, ErrCredentials
	}
	payload, e := base64.StdEncoding.DecodeString(envelope.Payload)
	var data struct {
		Version        int      `json:"version"`
		EdgePublic     string   `json:"edge_public_key"`
		MatrixPublic   string   `json:"matrix_public_key"`
		Address        string   `json:"address"`
		Endpoint       string   `json:"edge_endpoint"`
		MatrixEndpoint string   `json:"kore_play_endpoint"`
		AllowedIPs     []string `json:"allowed_ips"`
	}
	if e != nil || catalog.DecodeStrict(payload, &data) != nil || data.Version != 1 || data.EdgePublic != inv.EdgePublicKey || data.MatrixPublic != inv.MatrixPublicKey || data.Address != inv.Address {
		return CredentialStatus{}, ErrCredentials
	}
	settings := PeerSettings{Interface: local.Interface, ListenPort: local.ListenPort, Keepalive: local.Keepalive, Address: data.Address, EdgeEndpoint: data.Endpoint, KorePlayPublicKey: data.MatrixPublic, KorePlayEndpoint: data.MatrixEndpoint, AllowedIPs: data.AllowedIPs}
	if settings.Validate() != nil {
		return CredentialStatus{}, ErrPeerSettings
	}
	// Retrying the same consumed invitation does not overwrite concurrent edits.
	s.mu.Lock()
	same := s.data.Settings != nil && hashPeerSettings(*s.data.Settings) == hashPeerSettings(settings)
	s.mu.Unlock()
	if same {
		return s.Snapshot()
	}
	return s.SavePeer(settings, expected)
}
func hashPeerSettings(s PeerSettings) string {
	b, _ := json.Marshal(s)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
