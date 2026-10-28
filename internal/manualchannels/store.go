package manualchannels

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	srt "github.com/datarhei/gosrt"
	"kore-edge/internal/catalog"
	"kore-edge/internal/multicast"
	"kore-edge/internal/security"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sync"
)

var localID = regexp.MustCompile(`^local_[a-f0-9]{32}$`)

type state struct {
	Version  int      `json:"version"`
	Revision uint64   `json:"revision"`
	Sources  []Source `json:"sources"`
}
type Summary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	Category    string `json:"category"`
	Address     string `json:"address"`
	Protocol    string `json:"protocol"`
	Interface   string `json:"interface"`
	LocalIP     string `json:"local_ip"`
	Enabled     bool   `json:"enabled"`
}
type Inventory struct {
	Revision uint64    `json:"revision"`
	Channels []Summary `json:"channels"`
}

func New(directory string, fallback Catalog) (*Service, error) {
	if security.PrivateDirectory(directory, true) != nil {
		return nil, ErrUnavailable
	}
	s := &Service{directory: directory, fallback: fallback, data: state{Version: 1, Sources: []Source{}}, interfaces: multicast.Interfaces, lookup: lookupDefault, slots: make(chan struct{}, 2), transcodeSlots: make(chan struct{}, 2), dialSRT: srt.Dial, command: exec.CommandContext}
	s.client = s.httpClient()
	file := filepath.Join(directory, "channels.json")
	if _, e := os.Lstat(file); !os.IsNotExist(e) {
		b, e := security.ReadPrivateFile(file, 4*1024*1024)
		if e != nil || catalog.DecodeStrict(b, &s.data) != nil || s.data.Version != 1 || len(s.data.Sources) > 512 {
			return nil, ErrUnavailable
		}
		seen := map[string]bool{}
		for _, source := range s.data.Sources {
			if !localID.MatchString(source.ID) || seen[source.ID] {
				return nil, ErrUnavailable
			}
			if _, e := normalize(source); e != nil {
				return nil, ErrUnavailable
			}
			seen[source.ID] = true
		}
	}
	if err := s.loadPresentation(); err != nil {
		return nil, err
	}
	if err := s.loadProcessing(); err != nil {
		return nil, err
	}
	return s, nil
}

// sync.Mutex is deliberately shared by lookup/snapshot/persistence. A failed
// atomic write never replaces the current in-memory source set.
type protected struct{ mu sync.Mutex }

func (s *Service) Revision() uint64 { s.mu.Lock(); defer s.mu.Unlock(); return s.revisionLocked() }
func (s *Service) Inventory() Inventory {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Inventory{Revision: s.revisionLocked(), Channels: []Summary{}}
	for _, c := range s.data.Sources {
		u, _ := parse(c.URL)
		out.Channels = append(out.Channels, Summary{ID: c.ID, Name: s.displayName(c), Title: s.presentation.Entries[c.ID].Title, Description: s.presentation.Entries[c.ID].Description, Category: c.Category, Address: safeAddress(u), Protocol: c.Protocol, Interface: c.Interface, LocalIP: c.LocalIP, Enabled: c.Enabled})
	}
	return out
}
func (s *Service) source(id string) (Source, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, c := range s.data.Sources {
		if c.ID == id {
			c.Name = s.displayName(c)
			return c, true
		}
	}
	return Source{}, false
}
func (s *Service) Save(input Source, expected uint64) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.revisionLocked() != expected {
		return "", ErrConflict
	}
	index := -1
	for i, c := range s.data.Sources {
		if c.ID == input.ID {
			index = i
			if input.URL == "" {
				input.URL = c.URL
			}
			break
		}
	}
	if input.ID != "" && index < 0 {
		return "", ErrInvalid
	}
	next, e := normalize(input)
	if e != nil {
		return "", e
	}
	if next.Protocol == "udp" {
		if _, e = s.udpTarget(next); e != nil {
			return "", e
		}
	}
	data := s.data
	data.Sources = append([]Source{}, data.Sources...)
	if index < 0 {
		if len(data.Sources) >= 512 {
			return "", ErrConflict
		}
		key := make([]byte, 16)
		if _, e = rand.Read(key); e != nil {
			return "", ErrUnavailable
		}
		next.ID = "local_" + hex.EncodeToString(key)
		data.Sources = append(data.Sources, next)
	} else {
		data.Sources[index] = next
	}
	data.Revision++
	if e = s.persist(data); e != nil {
		return "", e
	}
	s.data = data
	return next.ID, nil
}
func (s *Service) Delete(id string, expected uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected != s.revisionLocked() {
		return ErrConflict
	}
	next := s.data
	next.Sources = []Source{}
	found := false
	for _, c := range s.data.Sources {
		if c.ID == id {
			found = true
		} else {
			next.Sources = append(next.Sources, c)
		}
	}
	if !found {
		return ErrInvalid
	}
	next.Revision++
	if e := s.persist(next); e != nil {
		return e
	}
	s.data = next
	return nil
}
func (s *Service) persist(next state) error { return s.persistFile("channels.json", next) }
func (s *Service) persistFile(name string, next any) error {
	b, e := json.Marshal(next)
	if e != nil || len(b) > 4*1024*1024 {
		return ErrInvalid
	}
	f, e := os.CreateTemp(s.directory, ".manual-")
	if e != nil {
		return ErrUnavailable
	}
	defer os.Remove(f.Name())
	if _, e = f.Write(b); e != nil {
		f.Close()
		return ErrUnavailable
	}
	if f.Sync() != nil {
		f.Close()
		return ErrUnavailable
	}
	if f.Close() != nil || os.Rename(f.Name(), filepath.Join(s.directory, name)) != nil {
		return ErrUnavailable
	}
	if os.PathSeparator != '\\' {
		dir, e := os.Open(s.directory)
		if e != nil {
			return ErrUnavailable
		}
		e = dir.Sync()
		dir.Close()
		if e != nil {
			return ErrUnavailable
		}
	}
	return nil
}
