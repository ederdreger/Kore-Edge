// Package operations prepares private, bounded releases for a future startup.
// It never stops, starts or updates a running process or calls host utilities.
package operations

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"kore-edge/internal/catalog"
	"kore-edge/internal/config"
	"kore-edge/internal/security"
)

const MaxBinaryBytes = 128 * 1024 * 1024
const MaxReleases = 8

var ErrStorage = errors.New("operations storage unavailable")
var ErrRejected = errors.New("operations request rejected")
var ErrConflict = errors.New("operations generation conflict")

type Manifest struct {
	Version      int    `json:"version"`
	ID           string `json:"id"`
	BinarySHA256 string `json:"binary_sha256"`
	ConfigSHA256 string `json:"config_sha256"`
}
type State struct {
	Version    int    `json:"version"`
	Generation uint64 `json:"generation"`
	Current    string `json:"current"`
	Previous   string `json:"previous"`
}
type Store struct {
	mu        sync.Mutex
	directory string
	lock      *os.File
}

func digest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
func validID(s string) bool {
	b, e := hex.DecodeString(s)
	return e == nil && len(b) == 32 && hex.EncodeToString(b) == s
}
func releaseID(binary, configuration string) string {
	return digest([]byte(binary + "\n" + configuration))
}

func Open(directory string) (*Store, error) {
	directory = filepath.Clean(directory)
	if security.PrivateDirectory(directory, true) != nil {
		return nil, ErrStorage
	}
	f, e := os.OpenFile(filepath.Join(directory, "operations.lock"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, ErrStorage
	}
	s := &Store{directory: directory, lock: f}
	return s, nil
}
func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil
	}
	e := s.lock.Close()
	s.lock = nil
	if os.Remove(filepath.Join(s.directory, "operations.lock")) != nil {
		return ErrStorage
	}
	return e
}

// readSource allows reviewed build/config input inside the checkout, but rejects
// symlinks, nonregular files, replacements between Lstat/Open and oversized data.
func readSource(path string, limit int64) ([]byte, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() > limit {
		return nil, ErrRejected
	}
	f, e := os.Open(path)
	if e != nil {
		return nil, ErrRejected
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return nil, ErrRejected
	}
	b, e := io.ReadAll(io.LimitReader(f, limit+1))
	if e != nil || int64(len(b)) > limit {
		return nil, ErrRejected
	}
	return b, nil
}
func writeNew(path string, b []byte, mode os.FileMode) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if e != nil {
		return ErrStorage
	}
	n, e := f.Write(b)
	if e != nil || n != len(b) {
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
	return nil
}
func (s *Store) usable() bool { return s != nil && s.lock != nil }
func (s *Store) Status() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.status()
}
func (s *Store) status() (State, error) {
	if !s.usable() {
		return State{}, ErrStorage
	}
	path := filepath.Join(s.directory, "selection.json")
	if _, e := os.Lstat(path); os.IsNotExist(e) {
		return State{Version: 1}, nil
	}
	b, e := security.ReadPrivateFile(path, 4096)
	if e != nil {
		return State{}, ErrStorage
	}
	var st State
	if catalog.DecodeStrict(b, &st) != nil || st.Version != 1 || !validID(st.Current) || st.Generation == 0 || st.Previous != "" && !validID(st.Previous) || st.Current == st.Previous {
		return State{}, ErrStorage
	}
	return st, nil
}

// Stage validates the copied bytes and expected binary digest before publishing
// an immutable revision. Configuration is its backup, never printed by the CLI.
func (s *Store) Stage(binaryPath, expectedSHA, configPath string) (Manifest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.usable() {
		return Manifest{}, ErrStorage
	}
	if !validID(expectedSHA) {
		return Manifest{}, ErrRejected
	}
	binary, e := readSource(binaryPath, MaxBinaryBytes)
	if e != nil || len(binary) == 0 || digest(binary) != expectedSHA {
		return Manifest{}, ErrRejected
	}
	configuration, e := readSource(configPath, config.MaxBytes)
	if e != nil {
		return Manifest{}, ErrRejected
	}
	if _, e := config.Parse(bytes.NewReader(configuration)); e != nil {
		return Manifest{}, ErrRejected
	}
	m := Manifest{Version: 1, BinarySHA256: expectedSHA, ConfigSHA256: digest(configuration)}
	m.ID = releaseID(m.BinarySHA256, m.ConfigSHA256)
	path := filepath.Join(s.directory, m.ID)
	if _, e := os.Lstat(path); e == nil {
		old, _, e := s.verify(m.ID)
		if e != nil || old != m {
			return Manifest{}, ErrStorage
		}
		return m, nil
	} else if !os.IsNotExist(e) {
		return Manifest{}, ErrStorage
	}
	entries, e := os.ReadDir(s.directory)
	if e != nil {
		return Manifest{}, ErrStorage
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".pending-") {
			count++
		}
	}
	if count >= MaxReleases {
		return Manifest{}, ErrStorage
	}
	pending, e := os.MkdirTemp(s.directory, ".pending-")
	if e != nil {
		return Manifest{}, ErrStorage
	}
	defer os.RemoveAll(pending)
	if os.Chmod(pending, 0700) != nil {
		return Manifest{}, ErrStorage
	}
	data, _ := json.Marshal(m)
	for _, item := range []struct {
		name string
		b    []byte
		mode os.FileMode
	}{{"agent", binary, 0700}, {"config.yaml", configuration, 0600}, {"manifest.json", data, 0600}} {
		if writeNew(filepath.Join(pending, item.name), item.b, item.mode) != nil {
			return Manifest{}, ErrStorage
		}
	}
	if syncDirectory(pending) != nil || os.Rename(pending, path) != nil || syncDirectory(s.directory) != nil {
		return Manifest{}, ErrStorage
	}
	return m, nil
}
func (s *Store) verify(id string) (Manifest, config.Config, error) {
	var m Manifest
	var c config.Config
	if !s.usable() || !validID(id) {
		return m, c, ErrRejected
	}
	directory := filepath.Join(s.directory, id)
	b, e := security.ReadPrivateFile(filepath.Join(directory, "manifest.json"), 4096)
	if e != nil || catalog.DecodeStrict(b, &m) != nil || m.Version != 1 || m.ID != id || !validID(m.BinarySHA256) || !validID(m.ConfigSHA256) || releaseID(m.BinarySHA256, m.ConfigSHA256) != id {
		return Manifest{}, c, ErrStorage
	}
	configuration, e := security.ReadPrivateFile(filepath.Join(directory, "config.yaml"), config.MaxBytes)
	if e != nil || digest(configuration) != m.ConfigSHA256 {
		return Manifest{}, c, ErrStorage
	}
	c, e = config.Parse(bytes.NewReader(configuration))
	if e != nil {
		return Manifest{}, config.Config{}, ErrStorage
	}
	binarySHA, e := security.PrivateFileSHA256(filepath.Join(directory, "agent"), MaxBinaryBytes)
	if e != nil || binarySHA != m.BinarySHA256 {
		return Manifest{}, config.Config{}, ErrStorage
	}
	return m, c, nil
}
func (s *Store) Select(expectedGeneration uint64, id string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selectRelease(expectedGeneration, id)
}
func (s *Store) selectRelease(expectedGeneration uint64, id string) (State, error) {
	st, e := s.status()
	if e != nil {
		return State{}, e
	}
	if st.Generation != expectedGeneration {
		return State{}, ErrConflict
	}
	if _, _, e := s.verify(id); e != nil {
		return State{}, e
	}
	if st.Current == id {
		return st, nil
	}
	if st.Generation == ^uint64(0) {
		return State{}, ErrRejected
	}
	next := State{Version: 1, Generation: st.Generation + 1, Current: id, Previous: st.Current}
	b, _ := json.Marshal(next)
	f, e := os.CreateTemp(s.directory, ".selection-")
	if e != nil {
		return State{}, ErrStorage
	}
	path := f.Name()
	defer os.Remove(path)
	if f.Chmod(0600) != nil {
		f.Close()
		return State{}, ErrStorage
	}
	n, e := f.Write(b)
	if e != nil || n != len(b) {
		f.Close()
		return State{}, ErrStorage
	}
	if f.Sync() != nil {
		f.Close()
		return State{}, ErrStorage
	}
	if f.Close() != nil || os.Rename(path, filepath.Join(s.directory, "selection.json")) != nil || syncDirectory(s.directory) != nil {
		return State{}, ErrStorage
	}
	return next, nil
}
func (s *Store) Rollback(expectedGeneration uint64) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, e := s.status()
	if e != nil {
		return State{}, e
	}
	if st.Generation != expectedGeneration {
		return State{}, ErrConflict
	}
	if st.Previous == "" {
		return State{}, ErrRejected
	}
	return s.selectRelease(expectedGeneration, st.Previous)
}

// LoadSelected is used only during agent startup. The selected binary must be
// the executable actually running; changing a pointer cannot hot-swap a process.
func LoadSelected(directory, executable string) (config.Config, error) {
	s, e := Open(directory)
	if e != nil {
		return config.Config{}, e
	}
	defer s.Close()
	st, e := s.status()
	if e != nil || st.Current == "" {
		return config.Config{}, ErrStorage
	}
	m, c, e := s.verify(st.Current)
	if e != nil {
		return config.Config{}, e
	}
	binarySHA, e := sourceSHA256(executable)
	if e != nil || binarySHA != m.BinarySHA256 {
		return config.Config{}, ErrRejected
	}
	return c, nil
}

func sourceSHA256(path string) (string, error) {
	info, e := os.Lstat(path)
	if e != nil || !info.Mode().IsRegular() || info.Size() > MaxBinaryBytes {
		return "", ErrRejected
	}
	f, e := os.Open(path)
	if e != nil {
		return "", ErrRejected
	}
	defer f.Close()
	opened, e := f.Stat()
	if e != nil || !os.SameFile(info, opened) {
		return "", ErrRejected
	}
	h := sha256.New()
	n, e := io.Copy(h, io.LimitReader(f, MaxBinaryBytes+1))
	if e != nil || n == 0 || n > MaxBinaryBytes {
		return "", ErrRejected
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
