// Package provision implements an offline laboratory enrollment authority.
// It never contacts a headend or applies network/service configuration.
package provision

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"path/filepath"
	"regexp"
	"time"
)

const MaxBytes = 256 * 1024
const MaxRecords = 128
const Domain = "kore-edge/enroll/v1\n"

var ErrRejected = errors.New("enrollment rejected")
var ErrStorage = errors.New("enrollment storage unavailable")
var ErrBusy = errors.New("enrollment storage busy")
var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

type Config struct {
	Enabled   bool   `yaml:"enabled"`
	EdgeID    string `yaml:"edge_id"`
	Directory string `yaml:"directory"`
}

func (c Config) Validate() error {
	if !c.Enabled {
		return nil
	}
	if !identifier.MatchString(c.EdgeID) || !filepath.IsAbs(c.Directory) || filepath.Dir(filepath.Clean(c.Directory)) == filepath.Clean(c.Directory) {
		return ErrRejected
	}
	return nil
}
func validCatalogKey(value string) bool {
	b, e := base64.StdEncoding.DecodeString(value)
	return e == nil && len(b) == ed25519.PublicKeySize && base64.StdEncoding.EncodeToString(b) == value
}
func deviceKey(value string) (*ecdsa.PublicKey, error) {
	b, e := base64.StdEncoding.DecodeString(value)
	if e != nil || len(b) > 256 || base64.StdEncoding.EncodeToString(b) != value {
		return nil, ErrRejected
	}
	parsed, e := x509.ParsePKIXPublicKey(b)
	if e != nil {
		return nil, ErrRejected
	}
	key, ok := parsed.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() || !key.Curve.IsOnCurve(key.X, key.Y) {
		return nil, ErrRejected
	}
	canonical, e := x509.MarshalPKIXPublicKey(key)
	if e != nil || !bytes.Equal(b, canonical) {
		return nil, ErrRejected
	}
	return key, nil
}

type Assignment struct {
	EdgeID           string `json:"edge_id"`
	PartnerID        string `json:"partner_id"`
	CatalogPublicKey string `json:"catalog_public_key"`
	TTLSeconds       int    `json:"ttl_seconds"`
}

func (a Assignment) validate() error {
	if !identifier.MatchString(a.EdgeID) || !identifier.MatchString(a.PartnerID) || !validCatalogKey(a.CatalogPublicKey) || a.TTLSeconds < 1 || a.TTLSeconds > 900 {
		return ErrRejected
	}
	return nil
}

type TokenInfo struct {
	ID        string    `json:"token_id"`
	ExpiresAt time.Time `json:"expires_at"`
}
type Credential struct {
	TokenInfo
	Token string `json:"token"`
}

// Explicit JSON delivery is required to obtain the bearer. Routine formatting
// and debug formatting must never turn a credential into a log line.
func (Credential) String() string   { return "Credential{token:REDACTED}" }
func (Credential) GoString() string { return "Credential{token:REDACTED}" }

// Challenge is a client-generated 32-byte nonce, base64url without padding.
// Sign Domain plus json.Marshal(Proof{Version, EdgeID, PublicKey, Challenge}).
type Proof struct {
	Version   int    `json:"version"`
	EdgeID    string `json:"edge_id"`
	PublicKey string `json:"public_key"`
	Challenge string `json:"challenge"`
}
type Request struct {
	Proof
	Signature string `json:"signature"`
}
type CatalogProposal struct {
	PartnerID string `json:"partner_id"`
	PublicKey string `json:"public_key"`
}
type Proposal struct {
	Version int             `json:"version"`
	Mode    string          `json:"mode"`
	Catalog CatalogProposal `json:"catalog"`
}
type Identity struct {
	EdgeID        string    `json:"edge_id"`
	PartnerID     string    `json:"partner_id"`
	PublicKey     string    `json:"public_key"`
	RegisteredAt  time.Time `json:"registered_at"`
	Configuration Proposal  `json:"configuration"`
}
type Status struct {
	State               string    `json:"state"`
	Identity            *Identity `json:"identity"`
	Applied             bool      `json:"configuration_applied"`
	PermissionsVerified bool      `json:"filesystem_permissions_verified"`
}
