package provision

import (
	"context"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"time"
)

func randomString(size int) (string, error) {
	b := make([]byte, size)
	if _, e := rand.Read(b); e != nil {
		return "", ErrStorage
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func tokenDigest(token string) ([32]byte, error) {
	if len(token) != 47 || token[:4] != "ke1_" {
		return [32]byte{}, ErrRejected
	}
	b, e := base64.RawURLEncoding.DecodeString(token[4:])
	if e != nil || len(b) != 32 || base64.RawURLEncoding.EncodeToString(b) != token[4:] {
		return [32]byte{}, ErrRejected
	}
	return sha256.Sum256([]byte(token)), nil
}

func (r *Registry) Issue(ctx context.Context, a Assignment, now time.Time) (Credential, error) {
	if a.validate() != nil || now.IsZero() {
		return Credential{}, ErrRejected
	}
	secret, e := randomString(32)
	if e != nil {
		return Credential{}, e
	}
	id, e := randomString(16)
	if e != nil {
		return Credential{}, e
	}
	id = "t_" + id
	token := "ke1_" + secret
	digest, _ := tokenDigest(token)
	info := TokenInfo{ID: id, ExpiresAt: now.Add(time.Duration(a.TTLSeconds) * time.Second)}
	e = r.withLock(ctx, func(db *database) (bool, error) {
		if len(db.Tokens) >= MaxRecords {
			return false, ErrStorage
		}
		for _, identity := range db.Identities {
			if identity.EdgeID == a.EdgeID {
				return false, ErrRejected
			}
		}
		db.Tokens = append(db.Tokens, tokenRecord{TokenInfo: info, Digest: hex.EncodeToString(digest[:]), Assignment: a, IssuedAt: now})
		return true, nil
	})
	if e != nil {
		return Credential{}, e
	}
	return Credential{TokenInfo: info, Token: token}, nil
}
func (r *Registry) Revoke(ctx context.Context, id string, now time.Time) error {
	if !identifier.MatchString(id) || now.IsZero() {
		return ErrRejected
	}
	return r.withLock(ctx, func(db *database) (bool, error) {
		for i := range db.Tokens {
			t := &db.Tokens[i]
			if t.ID == id {
				if t.RevokedAt != nil {
					return false, nil
				}
				stamp := now
				t.RevokedAt = &stamp
				return true, nil
			}
		}
		return false, ErrRejected
	})
}
func verifyProof(request Request) error {
	if request.Version != 1 || !identifier.MatchString(request.EdgeID) {
		return ErrRejected
	}
	nonce, e := base64.RawURLEncoding.DecodeString(request.Challenge)
	if e != nil || len(nonce) != 32 || base64.RawURLEncoding.EncodeToString(nonce) != request.Challenge {
		return ErrRejected
	}
	key, e := deviceKey(request.PublicKey)
	if e != nil {
		return ErrRejected
	}
	signature, e := base64.StdEncoding.DecodeString(request.Signature)
	if e != nil || len(signature) > 80 {
		return ErrRejected
	}
	payload, e := json.Marshal(request.Proof)
	digest := sha256.Sum256(append([]byte(Domain), payload...))
	if e != nil || !ecdsa.VerifyASN1(key, digest[:], signature) {
		return ErrRejected
	}
	return nil
}
func (r *Registry) Register(ctx context.Context, expectedEdge, token string, request Request, now time.Time) (Identity, error) {
	started := time.Now()
	if expectedEdge != request.EdgeID || verifyProof(request) != nil {
		return Identity{}, ErrRejected
	}
	digest, e := tokenDigest(token)
	if e != nil {
		return Identity{}, ErrRejected
	}
	var result Identity
	e = r.withLock(ctx, func(db *database) (bool, error) {
		// Expiry is checked at the serialized consumption point, not before
		// waiting for another process's lock. Preserve the injectable base time.
		consumedAt := now.Add(time.Since(started))
		index := -1
		for i, t := range db.Tokens {
			stored, e := hex.DecodeString(t.Digest)
			if e != nil {
				return false, ErrStorage
			}
			if subtle.ConstantTimeCompare(stored, digest[:]) == 1 {
				index = i
			}
		}
		if index < 0 {
			return false, ErrRejected
		}
		t := &db.Tokens[index]
		if t.Assignment.EdgeID != expectedEdge || t.RevokedAt != nil || t.UsedAt != nil || consumedAt.Before(t.IssuedAt) || !consumedAt.Before(t.ExpiresAt) {
			return false, ErrRejected
		}
		if len(db.Identities) >= MaxRecords {
			return false, ErrStorage
		}
		for _, identity := range db.Identities {
			if identity.EdgeID == expectedEdge {
				return false, ErrRejected
			}
		}
		result = Identity{EdgeID: expectedEdge, PartnerID: t.Assignment.PartnerID, PublicKey: request.PublicKey, RegisteredAt: consumedAt, Configuration: Proposal{Version: 1, Mode: "laboratory", Catalog: CatalogProposal{PartnerID: t.Assignment.PartnerID, PublicKey: t.Assignment.CatalogPublicKey}}}
		stamp := consumedAt
		t.UsedAt = &stamp
		db.Identities = append(db.Identities, result)
		return true, nil
	})
	if e != nil {
		return Identity{}, e
	}
	return result, nil
}
func (r *Registry) Status(ctx context.Context, edge string) (Status, error) {
	if !identifier.MatchString(edge) {
		return Status{}, ErrRejected
	}
	out := Status{State: "pending", PermissionsVerified: permissionsVerified()}
	e := r.withLock(ctx, func(db *database) (bool, error) {
		for _, identity := range db.Identities {
			if identity.EdgeID == edge {
				copy := identity
				out.State = "registered"
				out.Identity = &copy
				break
			}
		}
		return false, nil
	})
	if e != nil {
		return Status{}, e
	}
	return out, nil
}

func (r *Registry) PermissionsVerified() bool { return permissionsVerified() }
