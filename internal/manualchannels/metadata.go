package manualchannels

import (
	"kore-edge/internal/catalog"
	"kore-edge/internal/security"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

type presentation struct {
	Name        string `json:"name"`
	BaseName    string `json:"source_name"`
	Title       string `json:"title"`
	Description string `json:"description"`
}
type presentationState struct {
	Version  int                     `json:"version"`
	Revision uint64                  `json:"revision"`
	Entries  map[string]presentation `json:"entries"`
}

func validPresentation(p presentation) bool {
	return strings.TrimSpace(p.Name) != "" && len(p.Name) <= 128 && len(p.BaseName) <= 128 && len(p.Title) <= 256 && len(p.Description) <= 2048 && utf8.ValidString(p.Name+p.BaseName+p.Title+p.Description) && !strings.ContainsAny(p.Name+p.BaseName+p.Title, "\r\n\x00") && !strings.ContainsRune(p.Description, '\x00')
}
func (s *Service) loadPresentation() error {
	s.presentation = presentationState{Version: 1, Entries: map[string]presentation{}}
	file := filepath.Join(s.directory, "presentation.json")
	if _, err := os.Lstat(file); os.IsNotExist(err) {
		return nil
	}
	data, err := security.ReadPrivateFile(file, 4*1024*1024)
	if err != nil || catalog.DecodeStrict(data, &s.presentation) != nil || s.presentation.Version != 1 || s.presentation.Entries == nil || len(s.presentation.Entries) > 512 {
		return ErrUnavailable
	}
	for id, p := range s.presentation.Entries {
		if !localID.MatchString(id) || !validPresentation(p) {
			return ErrUnavailable
		}
	}
	return nil
}
func (s *Service) revisionLocked() uint64 { return s.data.Revision + s.presentation.Revision }
func (s *Service) displayName(source Source) string {
	if p, ok := s.presentation.Entries[source.ID]; ok && p.BaseName == source.Name {
		return p.Name
	}
	return source.Name
}

// Presentation has an independent atomic file. Origin URLs, activation and the
// legacy channels.json schema stay intact, including for binary rollback.
func (s *Service) SaveMetadata(id, name, title, description string, expected uint64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if expected != s.revisionLocked() {
		return ErrConflict
	}
	if !localID.MatchString(id) {
		return ErrInvalid
	}
	next := presentationState{Version: 1, Revision: s.presentation.Revision + 1, Entries: map[string]presentation{}}
	found := false
	for _, source := range s.data.Sources {
		if p, ok := s.presentation.Entries[source.ID]; ok {
			next.Entries[source.ID] = p
		}
		if source.ID == id {
			p := presentation{Name: strings.TrimSpace(name), BaseName: source.Name, Title: strings.TrimSpace(title), Description: strings.TrimSpace(description)}
			if !validPresentation(p) {
				return ErrInvalid
			}
			next.Entries[id] = p
			found = true
		}
	}
	if !found {
		return ErrInvalid
	}
	if err := s.persistFile("presentation.json", next); err != nil {
		return err
	}
	s.presentation = next
	return nil
}
