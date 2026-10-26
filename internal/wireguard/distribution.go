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
