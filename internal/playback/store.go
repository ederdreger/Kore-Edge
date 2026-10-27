// Package playback supplies stable scoped channel identities for external
// players, retaining verification of legacy temporary links during migration.
package playback

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"kore-edge/internal/catalog"
	"kore-edge/internal/security"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

var ErrDenied = errors.New("playback_denied")
var ErrStorage = errors.New("playback_storage_unavailable")
var safeID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,80}$`)
var ownerID = regexp.MustCompile(`^[a-f0-9]{32}$`)

const Lifetime = 7 * 24 * time.Hour

type Claims struct {
	Permanent  bool   `json:"-"`
	Channel    string `json:"c"`
	Owner      string `json:"o"`
	Revision   uint64 `json:"r"`
	Generation uint64 `json:"g"`
	Expires    int64  `json:"e"`
	HLS        bool   `json:"h"`
}
type state struct {
	Version   int               `json:"version"`
	Key       string            `json:"key"`
	Revisions map[string]uint64 `json:"revisions"`
}
type Store struct {
	mu        sync.Mutex
	directory string
	data      state
	key       []byte
	now       func() time.Time
	active    map[string]int
	total     int
}

func Open(directory string) (*Store, error) {
	if security.PrivateDirectory(directory, true) != nil {
		return nil, ErrStorage
	}
	s := &Store{directory: directory, now: time.Now, active: map[string]int{}}
	path := filepath.Join(directory, "playback.json")
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		key := make([]byte, 32)
		if _, e = rand.Read(key); e != nil {
			return nil, ErrStorage
		}
		s.data = state{Version: 1, Key: hex.EncodeToString(key), Revisions: map[string]uint64{}}
		if s.persist(s.data) != nil {
			return nil, ErrStorage
		}
	} else {
		data, e := security.ReadPrivateFile(path, 128*1024)
		if e != nil || catalog.DecodeStrict(data, &s.data) != nil {
			return nil, ErrStorage
		}
	}
	key, e := hex.DecodeString(s.data.Key)
	if e != nil || len(key) != 32 || s.data.Version != 1 || s.data.Revisions == nil || len(s.data.Revisions) > 512 {
		return nil, ErrStorage
	}
	for id := range s.data.Revisions {
		if !safeID.MatchString(id) {
			return nil, ErrStorage
		}
	}
	s.key = key
	return s, nil
}

func (s *Store) persist(next state) error {
	data, e := json.Marshal(next)
	if e != nil {
		return ErrStorage
	}
	f, e := os.CreateTemp(s.directory, ".playback-")
	if e != nil {
		return ErrStorage
	}
	name := f.Name()
	defer os.Remove(name)
	if _, e = f.Write(data); e != nil {
		f.Close()
		return ErrStorage
	}
	if e = f.Sync(); e != nil {
		f.Close()
		return ErrStorage
	}
	if f.Close() != nil {
		return ErrStorage
	}
	if os.Rename(name, filepath.Join(s.directory, "playback.json")) != nil {
		return ErrStorage
	}
	return nil
}

func (s *Store) Issue(id, owner string, revision uint64, hls bool) (string, time.Time, error) {
	if !safeID.MatchString(id) || !ownerID.MatchString(owner) || revision == 0 {
		return "", time.Time{}, ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expires := s.now().UTC().Add(Lifetime).Truncate(time.Second)
	data, _ := json.Marshal(Claims{Channel: id, Owner: owner, Revision: revision, Generation: s.data.Revisions[id], Expires: expires.Unix(), HLS: hls})
	payload := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), expires, nil
}

func (s *Store) Verify(raw, id string, hls bool) (Claims, error) {
	if len(raw) > 512 || !safeID.MatchString(id) {
		return Claims{}, ErrDenied
	}
	if strings.HasPrefix(raw, "p1.") {
		s.mu.Lock()
		defer s.mu.Unlock()
		expected := s.permanentLocked(id)
		if !hmac.Equal([]byte(raw), []byte(expected)) {
			return Claims{}, ErrDenied
		}
		return Claims{Channel: id, HLS: true, Permanent: true}, nil
	}

	parts := strings.Split(raw, ".")
	if len(parts) != 2 {
		return Claims{}, ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sig, e := base64.RawURLEncoding.DecodeString(parts[1])
	if e != nil {
		return Claims{}, ErrDenied
	}
	mac := hmac.New(sha256.New, s.key)
	mac.Write([]byte(parts[0]))
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return Claims{}, ErrDenied
	}
	data, e := base64.RawURLEncoding.DecodeString(parts[0])
	var c Claims
	if e != nil || catalog.DecodeStrict(data, &c) != nil || c.Channel != id || !ownerID.MatchString(c.Owner) || c.Revision == 0 || c.Generation != s.data.Revisions[id] || !s.now().Before(time.Unix(c.Expires, 0)) || hls && !c.HLS {
		return Claims{}, ErrDenied
	}
	return c, nil
}

func (s *Store) Revoke(ids []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := state{Version: 1, Key: s.data.Key, Revisions: map[string]uint64{}}
	for id, revision := range s.data.Revisions {
		next.Revisions[id] = revision
	}
	for _, id := range ids {
		if !safeID.MatchString(id) {
			return ErrDenied
		}
		next.Revisions[id]++
		if next.Revisions[id] == 0 {
			return ErrStorage
		}
	}
	if len(next.Revisions) > 512 {
		return ErrStorage
	}
	if s.persist(next) != nil {
		return ErrStorage
	}
	s.data = next
	return nil
}

// Bounded per-capability and global concurrency, also used for HLS requests.
func (s *Store) Acquire(raw string) (func(), bool) {
	sum := sha256.Sum256([]byte(raw))
	key := hex.EncodeToString(sum[:])
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.total >= 32 || s.active[key] >= 2 {
		return nil, false
	}
	s.total++
	s.active[key]++
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			s.total--
			s.active[key]--
			if s.active[key] == 0 {
				delete(s.active, key)
			}
		})
	}, true
}

// Permanent is deterministic for the channel lifetime, independent of accounts,
// clock, source URL and process restarts. Only channel deletion changes its key.
func (s *Store) Permanent(id string) (string, error) {
	if !safeID.MatchString(id) {
		return "", ErrDenied
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.permanentLocked(id), nil
}
func (s *Store) permanentLocked(id string) string {
	mac := hmac.New(sha256.New, s.key)
	data, _ := json.Marshal(struct {
		Domain     string
		ID         string
		Generation uint64
	}{"permanent-channel-v1", id, s.data.Revisions[id]})
	mac.Write(data)
	return "p1." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
