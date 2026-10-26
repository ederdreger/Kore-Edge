package wireguard

import (
	"crypto/ecdh"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"kore-edge/internal/security"
	"net/netip"
	"sort"
	"strings"
)

// ManagedConfig is read only by the explicitly installed privileged bridge.
// JSON deliberately excludes the private scalar, including from journal files.
type ManagedConfig struct {
	Interface  string `json:"interface"`
	Address    string `json:"address"`
	ListenPort int    `json:"listen_port"`
	PublicKey  string `json:"public_key"`
	Peers      []Peer `json:"peers"`
	Secret     string `json:"-"`
}

func (c ManagedConfig) Validate() error {
	p, e := netip.ParsePrefix(c.Address)
	if !namePattern.MatchString(c.Interface) || e != nil || !privatePrefix(p) || p.Bits() != p.Addr().BitLen() || c.Address != p.String() || c.ListenPort < 1 || c.ListenPort > 65535 || !validCurveKey(c.PublicKey) || len(c.Peers) < 1 || len(c.Peers) > 128 {
		return ErrPeerSettings
	}
	seen := map[string]bool{}
	var ranges []netip.Prefix
	for _, peer := range c.Peers {
		if !validCurveKey(peer.PublicKey) || peer.PublicKey == c.PublicKey || seen[peer.PublicKey] || (peer.Endpoint != "" && !validEndpoint(peer.Endpoint)) || peer.Keepalive < 0 || peer.Keepalive > 65535 || len(peer.AllowedIPs) < 1 || len(peer.AllowedIPs) > 32 {
			return ErrPeerSettings
		}
		seen[peer.PublicKey] = true
		for _, raw := range peer.AllowedIPs {
			route, e := netip.ParsePrefix(raw)
			if e != nil || route != route.Masked() || !privatePrefix(route) || route.Contains(p.Addr()) {
				return ErrPeerSettings
			}
			for _, old := range ranges {
				if old.Overlaps(route) {
					return ErrPeerSettings
				}
			}
			ranges = append(ranges, route)
		}
	}
	return nil
}
func (c ManagedConfig) Routes() []string {
	out := []string{}
	for _, p := range c.Peers {
		out = append(out, p.AllowedIPs...)
	}
	sort.Strings(out)
	return out
}
func (c ManagedConfig) WireGuardText() (string, error) {
	if c.Validate() != nil {
		return "", ErrPeerSettings
	}
	raw, e := base64.StdEncoding.DecodeString(c.Secret)
	if e != nil || len(raw) != 32 {
		return "", ErrCredentials
	}
	key, e := ecdh.X25519().NewPrivateKey(raw)
	if e != nil || base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()) != c.PublicKey {
		return "", ErrCredentials
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[Interface]\nPrivateKey = %s\nListenPort = %d\n", c.Secret, c.ListenPort)
	for _, p := range c.Peers {
		fmt.Fprintf(&b, "\n[Peer]\nPublicKey = %s\nAllowedIPs = %s\nPersistentKeepalive = %d\n", p.PublicKey, strings.Join(p.AllowedIPs, ", "), p.Keepalive)
		if p.Endpoint != "" {
			fmt.Fprintf(&b, "Endpoint = %s\n", p.Endpoint)
		}
	}
	return b.String(), nil
}

// ReadManaged supports only the two fixed application formats, never a browser
// supplied file path or arbitrary wg-quick directives/hooks.
func ReadManaged(path, role string) (ManagedConfig, error) {
	b, e := security.ReadPrivateFile(path, 1024*1024)
	if e != nil {
		return ManagedConfig{}, ErrCredentials
	}
	var out ManagedConfig
	declaredPublic := ""
	if role == "edge" {
		var data credentialFile
		if json.Unmarshal(b, &data) != nil || data.Version != 1 || data.Settings == nil {
			return out, ErrCredentials
		}
		s := data.Settings
		if s.Validate() != nil {
			return out, ErrPeerSettings
		}
		out = ManagedConfig{Interface: s.Interface, Address: s.Address, ListenPort: s.ListenPort, Secret: data.PrivateKey, Peers: []Peer{{PublicKey: s.KorePlayPublicKey, Endpoint: s.KorePlayEndpoint, AllowedIPs: s.AllowedIPs, Keepalive: s.Keepalive}}}
	} else if role == "matrix" {
		var data struct {
			Version  int `json:"version"`
			Identity struct {
				PrivateKey string `json:"private_key"`
				PublicKey  string `json:"public_key"`
			} `json:"identity"`
			Matrix struct {
				Interface  string `json:"interface"`
				Address    string `json:"address"`
				ListenPort int    `json:"listen_port"`
			} `json:"matrix"`
			Peers []struct {
				PublicKey string `json:"public_key"`
				Address   string `json:"address"`
				Endpoint  string `json:"endpoint"`
			} `json:"peers"`
		}
		if json.Unmarshal(b, &data) != nil || data.Version != 1 {
			return out, ErrCredentials
		}
		declaredPublic = data.Identity.PublicKey
		out = ManagedConfig{Interface: data.Matrix.Interface, Address: data.Matrix.Address, ListenPort: data.Matrix.ListenPort, Secret: data.Identity.PrivateKey}
		for _, peer := range data.Peers {
			out.Peers = append(out.Peers, Peer{PublicKey: peer.PublicKey, Endpoint: peer.Endpoint, AllowedIPs: []string{peer.Address}})
		}
	} else {
		return out, ErrPeerSettings
	}
	raw, e := base64.StdEncoding.DecodeString(out.Secret)
	if e != nil || len(raw) != 32 {
		return ManagedConfig{}, ErrCredentials
	}
	key, e := ecdh.X25519().NewPrivateKey(raw)
	if e != nil {
		return ManagedConfig{}, ErrCredentials
	}
	out.PublicKey = base64.StdEncoding.EncodeToString(key.PublicKey().Bytes())
	if (role == "matrix" && declaredPublic != out.PublicKey) || out.Validate() != nil {
		return ManagedConfig{}, ErrPeerSettings
	}
	sort.Slice(out.Peers, func(i, j int) bool { return out.Peers[i].PublicKey < out.Peers[j].PublicKey })
	return out, nil
}
