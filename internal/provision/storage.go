package provision

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kore-edge/internal/catalog"
)

type tokenRecord struct {
	TokenInfo
	Digest     string     `json:"digest"`
	Assignment Assignment `json:"assignment"`
	IssuedAt   time.Time  `json:"issued_at"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	UsedAt     *time.Time `json:"used_at,omitempty"`
}
type database struct {
	Version    int           `json:"version"`
	Revision   uint64        `json:"revision"`
	Tokens     []tokenRecord `json:"tokens"`
	Identities []Identity    `json:"identities"`
}
type Registry struct {
	directory string
	save      func(database) error
}

// PrivateDirectory rejects checkout-relative paths as well as a direct symlink.
// Call from the checkout or an installation work directory; runtime must live
// elsewhere. The operator must also keep it out of other repositories/backups.
func PrivateDirectory(path string) error {
	if !filepath.IsAbs(path) || filepath.Dir(filepath.Clean(path)) == filepath.Clean(path) {
		return ErrStorage
	}
	cwd, e := os.Getwd()
	if e != nil {
		return ErrStorage
	}
	rel, e := filepath.Rel(cwd, path)
	if e == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ErrStorage
	}
	return nil
}
func Open(directory string) (*Registry, error) {
	if PrivateDirectory(directory) != nil {
		return nil, ErrStorage
	}
	if os.MkdirAll(directory, 0700) != nil {
		return nil, ErrStorage
	}
	info, e := os.Lstat(directory)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || privatePermissions(info) != nil {
		return nil, ErrStorage
	}
	r := &Registry{directory: filepath.Clean(directory)}
	r.save = r.persist
	if e := r.withLock(context.Background(), func(*database) (bool, error) { return false, nil }); e != nil {
		return nil, e
	}
	return r, nil
}
func (r *Registry) load() (database, error) {
	path := filepath.Join(r.directory, "registry.json")
	info, e := os.Lstat(path)
	if os.IsNotExist(e) {
		return database{Version: 1, Tokens: []tokenRecord{}, Identities: []Identity{}}, nil
	}
	if e != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || privatePermissions(info) != nil || info.Size() > MaxBytes {
		return database{}, ErrStorage
	}
	f, e := os.Open(path)
	if e != nil {
		return database{}, ErrStorage
	}
	defer f.Close()
	b, e := io.ReadAll(io.LimitReader(f, MaxBytes+1))
	if e != nil || len(b) > MaxBytes {
		return database{}, ErrStorage
	}
	var db database
	if catalog.DecodeStrict(b, &db) != nil || db.Version != 1 || len(db.Tokens) > MaxRecords || len(db.Identities) > MaxRecords {
		return database{}, ErrStorage
	}
	ids, digests, edges := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, t := range db.Tokens {
		digest, e := hex.DecodeString(t.Digest)
		if !identifier.MatchString(t.ID) || ids[t.ID] || digests[t.Digest] || e != nil || len(digest) != 32 || t.Digest != hex.EncodeToString(digest) || t.Assignment.validate() != nil || t.IssuedAt.IsZero() || !t.ExpiresAt.After(t.IssuedAt) || t.ExpiresAt.Sub(t.IssuedAt) > 15*time.Minute {
			return database{}, ErrStorage
		}
		ids[t.ID] = true
		digests[t.Digest] = true
	}
	for _, id := range db.Identities {
		_, keyErr := deviceKey(id.PublicKey)
		if !identifier.MatchString(id.EdgeID) || !identifier.MatchString(id.PartnerID) || keyErr != nil || edges[id.EdgeID] || id.RegisteredAt.IsZero() || id.Configuration.Version != 1 || id.Configuration.Mode != "laboratory" || id.Configuration.Catalog.PartnerID != id.PartnerID || !validCatalogKey(id.Configuration.Catalog.PublicKey) {
			return database{}, ErrStorage
		}
		edges[id.EdgeID] = true
	}
	return db, nil
}
func (r *Registry) withLock(ctx context.Context, operation func(*database) (bool, error)) error {
	info, e := os.Lstat(r.directory)
	if e != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || privatePermissions(info) != nil {
		return ErrStorage
	}
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	lock := filepath.Join(r.directory, "registry.lock")
	for {
		if ctx.Err() != nil {
			return ErrBusy
		}
		f, e := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e == nil {
			f.Close()
			defer os.Remove(lock)
			break
		}
		if !os.IsExist(e) {
			return ErrStorage
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ErrBusy
		case <-timer.C:
		}
	}
	db, e := r.load()
	if e != nil {
		return e
	}
	dirty, e := operation(&db)
	if e != nil {
		return e
	}
	if !dirty {
		return nil
	}
	if db.Revision == ^uint64(0) {
		return ErrStorage
	}
	db.Revision++
	return r.save(db)
}
func (r *Registry) persist(db database) error {
	b, e := json.Marshal(db)
	if e != nil || len(b) > MaxBytes {
		return ErrStorage
	}
	f, e := os.CreateTemp(r.directory, ".registry-*")
	if e != nil {
		return ErrStorage
	}
	name := f.Name()
	defer os.Remove(name)
	if f.Chmod(0600) != nil {
		f.Close()
		return ErrStorage
	}
	if _, e = f.Write(b); e != nil {
		f.Close()
		return ErrStorage
	}
	if f.Sync() != nil {
		f.Close()
		return ErrStorage
	}
	if f.Close() != nil {
		return ErrStorage
	}
	if os.Rename(name, filepath.Join(r.directory, "registry.json")) != nil {
		return ErrStorage
	}
	return syncDirectory(r.directory)
}
