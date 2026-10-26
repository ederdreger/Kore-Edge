// Package catalog verifies partner-scoped, signed laboratory manifests.
package catalog

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"sync"
	"time"
)

const MaxBytes = 256 * 1024
const Domain = "kore-edge/catalog/v1\n"

var ErrRejected = errors.New("catalog rejected")
var ErrUnavailable = errors.New("catalog unavailable")
var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type Channel struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Source  string   `json:"source"`
	Outputs []string `json:"outputs"`
}
type Grant struct {
	ChannelID string   `json:"channel_id"`
	Outputs   []string `json:"outputs"`
}
type Manifest struct {
	Version   int       `json:"version"`
	Revision  uint64    `json:"revision"`
	PartnerID string    `json:"partner_id"`
	IssuedAt  time.Time `json:"issued_at"`
	ExpiresAt time.Time `json:"expires_at"`
	Channels  []Channel `json:"channels"`
	Grants    []Grant   `json:"grants"`
}

// Payload and signature are base64. Signature covers Domain plus exact payload
// bytes, not a JSON reserialization. The trusted public key is never in this envelope.
type Envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// DecodeStrict rejects ambiguous duplicate keys, unknown fields and extra documents.
func DecodeStrict(data []byte, dest any) error {
	if len(data) > MaxBytes {
		return ErrRejected
	}
	scan := json.NewDecoder(bytes.NewReader(data))
	var walk func(int) error
	walk = func(depth int) error {
		if depth > 32 {
			return ErrRejected
		}
		token, e := scan.Token()
		if e != nil {
			return e
		}
		delim, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		if delim == '{' {
			seen := map[string]bool{}
			for scan.More() {
				key, e := scan.Token()
				if e != nil {
					return e
				}
				s, ok := key.(string)
				if !ok || seen[s] {
					return ErrRejected
				}
				seen[s] = true
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			end, e := scan.Token()
			if e != nil || end != json.Delim('}') {
				return ErrRejected
			}
			return nil
		}
		if delim == '[' {
			for scan.More() {
				if e = walk(depth + 1); e != nil {
					return e
				}
			}
			end, e := scan.Token()
			if e != nil || end != json.Delim(']') {
				return ErrRejected
			}
			return nil
		}
		return ErrRejected
	}
	if walk(0) != nil {
		return ErrRejected
	}
	if _, e := scan.Token(); e != io.EOF {
		return ErrRejected
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(dest) != nil {
		return ErrRejected
	}
	return nil
}

func outputsValid(values []string) bool {
	seen := map[string]bool{}
	if len(values) == 0 || len(values) > 2 {
		return false
	}
	for _, v := range values {
		if (v != "udp" && v != "hls") || seen[v] {
			return false
		}
		seen[v] = true
	}
	return true
}
func (m Manifest) validate(partner string, now time.Time) error {
	if m.Version != 1 || m.Revision == 0 || m.PartnerID != partner || m.IssuedAt.IsZero() || m.IssuedAt.After(now) || !m.ExpiresAt.After(now) || !m.ExpiresAt.After(m.IssuedAt) || m.ExpiresAt.Sub(m.IssuedAt) > 24*time.Hour || len(m.Channels) > 512 || len(m.Grants) > 512 {
		return ErrRejected
	}
	grants := map[string]map[string]bool{}
	for _, g := range m.Grants {
		if !identifier.MatchString(g.ChannelID) || grants[g.ChannelID] != nil || !outputsValid(g.Outputs) {
			return ErrRejected
		}
		grants[g.ChannelID] = map[string]bool{}
		for _, v := range g.Outputs {
			grants[g.ChannelID][v] = true
		}
	}
	seen := map[string]bool{}
	for _, c := range m.Channels {
		if !identifier.MatchString(c.ID) || seen[c.ID] || len(c.Name) == 0 || len(c.Name) > 128 || c.Source != "synthetic://"+c.ID || !outputsValid(c.Outputs) {
			return ErrRejected
		}
		seen[c.ID] = true
		for _, v := range c.Outputs {
			if !grants[c.ID][v] {
				return ErrRejected
			}
		}
	}
	for id := range grants {
		if !seen[id] {
			return ErrRejected
		}
	}
	return nil
}

type Store struct {
	mu       sync.RWMutex
	key      ed25519.PublicKey
	partner  string
	manifest Manifest
	payload  []byte
	digest   [32]byte
	valid    bool
}

func New(partner, keyBase64 string) (*Store, error) {
	key, e := base64.StdEncoding.DecodeString(keyBase64)
	if !identifier.MatchString(partner) || e != nil || len(key) != ed25519.PublicKeySize {
		return nil, ErrRejected
	}
	return &Store{partner: partner, key: append(ed25519.PublicKey(nil), key...)}, nil
}

func (s *Store) Apply(data []byte, now time.Time) error {
	var env Envelope
	if DecodeStrict(data, &env) != nil {
		return ErrRejected
	}
	payload, e := base64.StdEncoding.DecodeString(env.Payload)
	if e != nil || len(payload) > MaxBytes {
		return ErrRejected
	}
	sig, e := base64.StdEncoding.DecodeString(env.Signature)
	if e != nil || !ed25519.Verify(s.key, append([]byte(Domain), payload...), sig) {
		return ErrRejected
	}
	var m Manifest
	if DecodeStrict(payload, &m) != nil || m.validate(s.partner, now) != nil {
		return ErrRejected
	}
	digest := sha256.Sum256(payload)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.valid && (m.Revision < s.manifest.Revision || (m.Revision == s.manifest.Revision && digest != s.digest)) {
		return ErrRejected
	}
	s.manifest = m
	s.payload = append([]byte(nil), payload...)
	s.digest = digest
	s.valid = true
	return nil
}

func (s *Store) Snapshot(now time.Time) (Manifest, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.valid || !s.manifest.ExpiresAt.After(now) || s.manifest.IssuedAt.After(now) {
		return Manifest{}, ErrUnavailable
	}
	// Decode fresh slices so callers cannot mutate the trusted snapshot.
	var m Manifest
	if json.Unmarshal(s.payload, &m) != nil {
		return Manifest{}, ErrUnavailable
	}
	return m, nil
}
func (s *Store) Authorize(channel, output string, now time.Time) bool {
	// Called per packet by UDP output: read validated state under the lock
	// without decoding/copying the whole manifest on the data path.
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.valid || !s.manifest.ExpiresAt.After(now) || s.manifest.IssuedAt.After(now) {
		return false
	}
	for _, c := range s.manifest.Channels {
		if c.ID == channel {
			for _, v := range c.Outputs {
				if v == output {
					return true
				}
			}
		}
	}
	return false
}
