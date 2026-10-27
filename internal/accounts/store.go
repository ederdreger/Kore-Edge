// Package accounts implements local panel identities and revocable sessions.
package accounts

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
	"kore-edge/internal/catalog"
	"kore-edge/internal/security"
)

var ErrRejected = errors.New("account request rejected")
var ErrBusy = errors.New("channel operation in progress")
var ErrStorage = errors.New("account storage unavailable")

type Config struct {
	Enabled      bool   `yaml:"enabled"`
	Directory    string `yaml:"directory"`
	PublicOrigin string `yaml:"public_origin"`
}

func (c Config) Validate() error {
	if !c.Enabled {
		if c.Directory != "" || c.PublicOrigin != "" {
			return ErrRejected
		}
		return nil
	}
	if !filepath.IsAbs(c.Directory) || canonicalOrigin(c.PublicOrigin) == "" {
		return ErrRejected
	}
	return nil
}

type User struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	Disabled bool   `json:"disabled"`
	Revision uint64 `json:"revision"`
}
type record struct {
	User
	Hash string `json:"password_hash"`
}
type database struct {
	Version int      `json:"version"`
	Users   []record `json:"users"`
}
type session struct {
	UserID, CSRF      string
	Revision          uint64
	Expires, LastSeen time.Time
	Idle              time.Duration
}
type Store struct {
	channelMu sync.Mutex
	mu        sync.Mutex
	directory string
	db        database
	lock      *os.File
	audit     *security.Audit
	sessions  map[string]session
	failures  map[string]int
	window    time.Time
	dummy     []byte
	slots     chan struct{}
	closed    bool
}

var idPattern = regexp.MustCompile(`^[a-f0-9]{32}$`)

var usernamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.@+-]{0,127}$`)

func token() string {
	var b [32]byte
	if _, e := rand.Read(b[:]); e != nil {
		return ""
	}
	return hex.EncodeToString(b[:])
}
func validPassword(p string) bool {
	return len(p) >= 12 && len(p) <= 72 && !strings.ContainsAny(p, "\r\n\x00")
}
func Open(directory string) (*Store, error) {
	if security.PrivateDirectory(directory, true) != nil {
		return nil, ErrStorage
	}
	lock, e := acquireLock(filepath.Join(directory, "users.lock"))
	if e != nil {
		return nil, ErrStorage
	}
	s := &Store{directory: directory, lock: lock, db: database{Version: 1, Users: []record{}}, sessions: map[string]session{}, failures: map[string]int{}, slots: make(chan struct{}, 4)}
	good := false
	defer func() {
		if !good {
			s.Close()
		}
	}()
	if _, e := os.Lstat(filepath.Join(directory, "users.json")); !os.IsNotExist(e) {
		b, e := security.ReadPrivateFile(filepath.Join(directory, "users.json"), 128*1024)
		if e != nil || catalog.DecodeStrict(b, &s.db) != nil || s.validate() != nil {
			return nil, ErrStorage
		}
	}
	// The kernel lock excludes any live writer; recover a stale audit lock after a crash.
	if e := os.Remove(filepath.Join(directory, "audit", "audit.lock")); e != nil && !os.IsNotExist(e) {
		return nil, ErrStorage
	}
	s.audit, e = security.OpenAudit(filepath.Join(directory, "audit"))
	if e != nil {
		return nil, ErrStorage
	}
	s.dummy, e = bcrypt.GenerateFromPassword([]byte(token()), 12)
	if e != nil {
		return nil, ErrStorage
	}
	good = true
	return s, nil
}
func (s *Store) validate() error {
	if s.db.Version != 1 || len(s.db.Users) > 64 {
		return ErrStorage
	}
	names := map[string]bool{}
	ids := map[string]bool{}
	admins := 0
	for _, u := range s.db.Users {
		if !usernamePattern.MatchString(u.Username) || names[strings.ToLower(u.Username)] || !idPattern.MatchString(u.ID) || ids[u.ID] || u.Revision == 0 || (u.Role != "admin" && u.Role != "viewer") {
			return ErrStorage
		}
		cost, e := bcrypt.Cost([]byte(u.Hash))
		if e != nil || cost < 10 || cost > 14 {
			return ErrStorage
		}
		names[strings.ToLower(u.Username)] = true
		ids[u.ID] = true
		if u.Role == "admin" && !u.Disabled {
			admins++
		}
	}
	if len(s.db.Users) > 0 && admins == 0 {
		return ErrStorage
	}
	return nil
}
func (s *Store) save(next database, actor, role, action string) (result error) {
	nonce := token()
	if len(nonce) < 16 {
		return ErrStorage
	}
	end, e := s.audit.Begin(security.Event{Time: time.Now().UTC(), RequestID: nonce[:16], PrincipalID: actor, Role: role, Route: action, Method: "POST"})
	if e != nil {
		return ErrStorage
	}
	status := 500
	defer func() {
		if end(status) != nil {
			result = ErrStorage
		}
	}()
	b, e := json.Marshal(next)
	if e != nil {
		return ErrStorage
	}
	f, e := os.CreateTemp(s.directory, ".users-")
	if e != nil {
		return ErrStorage
	}
	path := f.Name()
	defer os.Remove(path)
	if f.Chmod(0600) != nil {
		f.Close()
		return ErrStorage
	}
	if n, e := f.Write(b); e != nil || n != len(b) {
		f.Close()
		return ErrStorage
	}
	if f.Sync() != nil {
		f.Close()
		return ErrStorage
	}
	if f.Close() != nil || os.Rename(path, filepath.Join(s.directory, "users.json")) != nil {
		return ErrStorage
	}
	s.db = next
	status = 204
	return syncDirectory(s.directory)
}

// BootstrapHash imports the existing administrator's bcrypt hash without needing
// their plaintext password. It can only initialize an empty identity store.
func (s *Store) BootstrapHash(username, hash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cost, e := bcrypt.Cost([]byte(hash))
	if s.closed || len(s.db.Users) != 0 || !usernamePattern.MatchString(username) || e != nil || cost < 10 || cost > 14 {
		return ErrRejected
	}
	id := token()
	if len(id) != 64 {
		return ErrStorage
	}
	u := record{User: User{ID: id[:32], Username: username, Role: "admin", Revision: 1}, Hash: hash}
	return s.save(database{Version: 1, Users: []record{u}}, "bootstrap", "admin", "account_bootstrap")
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.audit != nil {
		s.audit.Close()
	}
	if s.lock != nil {
		s.lock.Close()

	}
	return nil
}
func (s *Store) Users() []User {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := []User{}
	for _, u := range s.db.Users {
		out = append(out, u.User)
	}
	return out
}
func (s *Store) login(username, password string, remember bool) (string, session, User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	name := strings.ToLower(strings.TrimSpace(username))
	if now.Sub(s.window) > time.Minute {
		s.failures = map[string]int{}
		s.window = now
	}
	if s.failures["*"] >= 30 || s.failures[name] >= 5 {
		return "", session{}, User{}, ErrRejected
	}
	s.failures["*"]++
	s.failures[name]++
	selected := -1
	hash := s.dummy
	for i, u := range s.db.Users {
		if strings.EqualFold(u.Username, name) && !u.Disabled {
			selected = i
			hash = []byte(u.Hash)
		}
	}
	if s.closed || bcrypt.CompareHashAndPassword(hash, []byte(password)) != nil || selected < 0 {
		return "", session{}, User{}, ErrRejected
	}
	for key, v := range s.sessions {
		if now.After(v.Expires) || now.Sub(v.LastSeen) > v.Idle {
			delete(s.sessions, key)
		}
	}
	if len(s.sessions) >= 256 {
		return "", session{}, User{}, ErrRejected
	}
	raw, csrf := token(), token()
	if raw == "" || csrf == "" {
		return "", session{}, User{}, ErrStorage
	}
	u := s.db.Users[selected].User
	duration, idle := 8*time.Hour, 30*time.Minute
	if remember {
		duration, idle = 7*24*time.Hour, 24*time.Hour
	}
	v := session{UserID: u.ID, Revision: u.Revision, CSRF: csrf, Expires: now.Add(duration), LastSeen: now, Idle: idle}
	s.sessions[sessionKey(raw)] = v
	delete(s.failures, name)
	return raw, v, u, nil
}
func (s *Store) authenticate(raw string) (session, User, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := sessionKey(raw)
	v, ok := s.sessions[key]
	now := time.Now().UTC()
	if !ok || s.closed || now.After(v.Expires) || now.Sub(v.LastSeen) > v.Idle {
		delete(s.sessions, key)
		return session{}, User{}, false
	}
	for _, u := range s.db.Users {
		if u.ID == v.UserID && u.Revision == v.Revision && !u.Disabled {
			v.LastSeen = now
			s.sessions[key] = v
			return v, u.User, true
		}
	}
	delete(s.sessions, key)
	return session{}, User{}, false
}
func (s *Store) change(actor User, actorPassword, id, username, password, role string, disabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	a := -1
	for i, u := range s.db.Users {
		if u.ID == actor.ID && !u.Disabled && u.Revision == actor.Revision {
			a = i
		}
	}
	if a < 0 || bcrypt.CompareHashAndPassword([]byte(s.db.Users[a].Hash), []byte(actorPassword)) != nil {
		return ErrRejected
	}
	next := database{Version: 1, Users: append([]record(nil), s.db.Users...)}
	index := -1
	for i, u := range next.Users {
		if u.ID == id {
			index = i
		}
	}
	if id == "" {
		if actor.Role != "admin" || len(next.Users) >= 64 || !usernamePattern.MatchString(username) || !validPassword(password) || (role != "admin" && role != "viewer") {
			return ErrRejected
		}
		for _, u := range next.Users {
			if strings.EqualFold(u.Username, username) {
				return ErrRejected
			}
		}
		raw := token()
		if raw == "" {
			return ErrStorage
		}
		next.Users = append(next.Users, record{User: User{ID: raw[:32], Username: username, Role: role, Revision: 1}})
		index = len(next.Users) - 1
	} else {
		if index < 0 {
			return ErrRejected
		}
		u := next.Users[index]
		if actor.Role != "admin" && (actor.ID != id || role != u.Role || disabled != u.Disabled) {
			return ErrRejected
		}
		if role != "admin" && role != "viewer" {
			return ErrRejected
		}
		next.Users[index].Role = role
		next.Users[index].Disabled = disabled
		next.Users[index].Revision++
	}
	if password != "" {
		if !validPassword(password) {
			return ErrRejected
		}
		hash, e := bcrypt.GenerateFromPassword([]byte(password), 12)
		if e != nil {
			return ErrStorage
		}
		next.Users[index].Hash = string(hash)
	}
	admins := 0
	for _, u := range next.Users {
		if u.Role == "admin" && !u.Disabled {
			admins++
		}
	}
	if admins == 0 {
		return ErrRejected
	}
	return s.save(next, actor.ID, actor.Role, "account_change")
}

// Directory is the immutable private storage path used by optional local modules.
func (s *Store) Directory() string { return s.directory }
