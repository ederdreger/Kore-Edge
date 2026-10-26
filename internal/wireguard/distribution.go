package wireguard

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"
)

func (s *CredentialStore) distributionSecret() ([]byte, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.data.Settings == nil {
		return nil, "", ErrCredentials
	}
	raw, e := base64.StdEncoding.DecodeString(s.data.PrivateKey)
	if e != nil {
		return nil, "", ErrCredentials
	}
	private, e := ecdh.X25519().NewPrivateKey(raw)
	if e != nil {
		return nil, "", ErrCredentials
	}
	remote, e := base64.StdEncoding.DecodeString(s.data.Settings.KorePlayPublicKey)
	if e != nil {
		return nil, "", ErrCredentials
	}
	public, e := ecdh.X25519().NewPublicKey(remote)
	if e != nil {
		return nil, "", ErrCredentials
	}
	shared, e := private.ECDH(public)
	if e != nil {
		return nil, "", ErrCredentials
	}
	return shared, s.publicKey(), nil
}

// MatchesDistribution checks the pinned identity without constructing a panel
// DTO or repeating X25519 on every transport packet. Revision changes invalidate
// the public-key cache; no private material is cached or returned here.
func (s *CredentialStore) MatchesDistribution(edge, matrix string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.data.Settings == nil {
		return false
	}
	if s.distributionRevision != s.data.Revision || s.distributionPublic == "" {
		s.distributionPublic = s.publicKey()
		s.distributionRevision = s.data.Revision
	}
	return s.distributionPublic != "" && edge == s.distributionPublic && matrix == s.data.Settings.KorePlayPublicKey
}

// DistributionHeaders proves possession of the existing Edge identity. Neither
// private key nor shared secret is exported in an HTTP request or browser DTO.
func (s *CredentialStore) DistributionHeaders(path string) (http.Header, error) {
	key, public, e := s.distributionSecret()
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, 16)
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	stamp := strconv.FormatInt(time.Now().Unix(), 10)
	n := hex.EncodeToString(nonce)
	headers := http.Header{}
	headers.Set("X-Kore-Edge-Features", "live-v1,srt-v1")
	headers.Set("X-Kore-Edge-Key", public)
	headers.Set("X-Kore-Edge-Time", stamp)
	headers.Set("X-Kore-Edge-Nonce", n)
	headers.Set("X-Kore-Edge-Proof", sign(key, "kore-edge/distribution/request/v1\nGET\n"+path+"\n"+stamp+"\n"+n))
	return headers, nil
}
func (s *CredentialStore) VerifyDistribution(nonce, payload, proof string) bool {
	key, _, e := s.distributionSecret()
	if e != nil {
		return false
	}
	want, _ := hex.DecodeString(sign(key, "kore-edge/distribution/reply/v1\n"+nonce+"\n"+payload))
	got, e := hex.DecodeString(proof)
	return e == nil && hmac.Equal(want, got)
}

// SRTSecret stays inside server-side transport code, never in API responses,
// process arguments, versioned files or logs.
func (s *CredentialStore) SRTSecret(partnerID string) (string, error) {
	key, _, e := s.distributionSecret()
	if e != nil {
		return "", e
	}
	if len(partnerID) != 32 {
		return "", ErrCredentials
	}
	if _, e = hex.DecodeString(partnerID); e != nil {
		return "", ErrCredentials
	}
	return sign(key, "kore-edge/srt/encryption/v1\n"+partnerID)[:32], nil
}
