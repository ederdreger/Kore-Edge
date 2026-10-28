package manualchannels

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestPresentationPersistsWithoutRewritingLegacyOriginsOrCredentials(t *testing.T) {
	s := fixture(t)
	url := "https://synthetic-user:synthetic-password@media.example/CHANNEL/mpegts?token=synthetic-token"
	id, err := s.Save(Source{Name: "Original", URL: url, Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.directory, "channels.json")
	original, _ := os.ReadFile(file)
	if err = s.SaveMetadata(id, "Nome novo", "Título sintético", "Descrição\ncom duas linhas", 1); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(file)
	if !bytes.Equal(original, after) {
		t.Fatal("metadata rewrote the origin file")
	}
	inventory := s.Inventory()
	if inventory.Revision != 2 || inventory.Channels[0].Name != "Nome novo" || inventory.Channels[0].Title != "Título sintético" {
		t.Fatal("presentation missing", inventory)
	}
	if strings.Contains(inventory.Channels[0].Address, "synthetic-password") {
		t.Fatal("credential leak")
	}
	copy, err := New(s.directory, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer copy.Close()
	source, ok := copy.source(id)
	if !ok || source.URL != url || !source.Enabled || source.Name != "Nome novo" {
		t.Fatal("origin changed or presentation not reloaded")
	}
	// The existing editor can save origin settings without dropping new metadata.
	if _, err = s.Save(Source{ID: id, Name: "Nome novo", Protocol: "http_ts", Enabled: true}, 2); err != nil {
		t.Fatal(err)
	}
	if s.Inventory().Channels[0].Title != "Título sintético" {
		t.Fatal("legacy origin edit lost the title")
	}
	if err = s.SaveMetadata(id, "Nome novo", "", "", 3); err != nil {
		t.Fatal(err)
	}
	if s.Inventory().Channels[0].Title != "" || s.Inventory().Channels[0].Description != "" {
		t.Fatal("clearing metadata failed")
	}
}
func TestPresentationRejectsStaleAndInvalidWritesWithoutChangingRevision(t *testing.T) {
	s := fixture(t)
	id, err := s.Save(Source{Name: "Synthetic", URL: "https://media.example/live/mpegts", Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveMetadata(id, "Updated", "", "", 0); err != ErrConflict {
		t.Fatal("stale metadata accepted", err)
	}
	for _, p := range []presentation{{Name: ""}, {Name: "bad\nname"}, {Name: "valid", Title: strings.Repeat("x", 257)}, {Name: "valid", Description: strings.Repeat("x", 2049)}, {Name: "valid", Description: "bad\x00text"}} {
		if err = s.SaveMetadata(id, p.Name, p.Title, p.Description, 1); err != ErrInvalid {
			t.Fatal("invalid metadata accepted", err)
		}
		if s.Revision() != 1 {
			t.Fatal("invalid operation changed revision")
		}
	}
}

func TestPresentationFileRemainsPrivate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permission boundary")
	}
	s := fixture(t)
	id, err := s.Save(Source{Name: "Synthetic", URL: "https://media.example/live/mpegts", Enabled: true}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.SaveMetadata(id, "Synthetic", "Title", "Description", 1); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(s.directory, "presentation.json")
	info, err := os.Stat(file)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0077 != 0 {
		t.Fatal("presentation file readable by other users")
	}
	if err = os.Chmod(file, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err = New(s.directory, nil); err == nil {
		t.Fatal("public presentation file accepted")
	}
}
