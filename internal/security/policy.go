package security

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"regexp"
	"time"

	"kore-edge/internal/catalog"
)

type Client struct {
	ID          string `json:"id"`
	Fingerprint string `json:"certificate_sha256"`
	Role        string `json:"role"`
	Disabled    bool   `json:"disabled"`
}
type Policy struct {
	Version int      `json:"version"`
	Clients []Client `json:"clients"`
}

var opaque = regexp.MustCompile(`^[a-f0-9]{32}$`)

func validRole(role string) bool {
	return role == "viewer" || role == "operator" || role == "admin" || role == "enroller" || role == "metrics"
}
func loadPolicy(path string) (Policy, error) {
	b, e := readPrivate(path, 64*1024)
	if e != nil {
		return Policy{}, e
	}
	var p Policy
	if catalog.DecodeStrict(b, &p) != nil || p.Version != 1 || len(p.Clients) == 0 || len(p.Clients) > 64 {
		return Policy{}, ErrUnavailable
	}
	ids, fingerprints := map[string]bool{}, map[string]bool{}
	for _, c := range p.Clients {
		raw, e := hex.DecodeString(c.Fingerprint)
		if !opaque.MatchString(c.ID) || ids[c.ID] || e != nil || len(raw) != 32 || hex.EncodeToString(raw) != c.Fingerprint || fingerprints[c.Fingerprint] || !validRole(c.Role) {
			return Policy{}, ErrUnavailable
		}
		ids[c.ID] = true
		fingerprints[c.Fingerprint] = true
	}
	return p, nil
}
func (p Policy) authenticate(state *tls.ConnectionState, now time.Time) (Client, error) {
	if state == nil || state.Version < tls.VersionTLS13 || len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 {
		return Client{}, ErrDenied
	}
	leaf := state.PeerCertificates[0]
	if now.Before(leaf.NotBefore) || !now.Before(leaf.NotAfter) {
		return Client{}, ErrDenied
	}
	for _, chain := range state.VerifiedChains {
		for _, cert := range chain {
			if now.Before(cert.NotBefore) || !now.Before(cert.NotAfter) {
				return Client{}, ErrDenied
			}
		}
	}
	digest := sha256.Sum256(leaf.Raw)
	fingerprint := hex.EncodeToString(digest[:])
	for _, c := range p.Clients {
		if c.Fingerprint == fingerprint && !c.Disabled {
			return c, nil
		}
	}
	return Client{}, ErrDenied
}
