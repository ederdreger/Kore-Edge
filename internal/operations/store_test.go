package operations

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

const testConfig = "version: 1\nmode: laboratory\nhttp:\n  listen: '127.0.0.1:18080'\n"

func writeFixture(t *testing.T, path string, b []byte) {
	t.Helper()
	if e := os.WriteFile(path, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func fixture(t *testing.T) (*Store, string, string) {
	t.Helper()
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	s, e := Open(filepath.Join(dir, "releases"))
	if e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { s.Close() })
	binary := filepath.Join(dir, "source-agent")
	configuration := filepath.Join(dir, "source.yaml")
	writeFixture(t, binary, []byte("synthetic agent 1"))
	writeFixture(t, configuration, []byte(testConfig))
	return s, binary, configuration
}
func stageFixture(t *testing.T, s *Store, binary, configuration string) Manifest {
	t.Helper()
	b, e := os.ReadFile(binary)
	if e != nil {
		t.Fatal(e)
	}
	m, e := s.Stage(binary, digest(b), configuration)
	if e != nil {
		t.Fatal(e)
	}
	return m
}
func TestStageBackupSelectRollbackAndStartup(t *testing.T) {
	s, binary, configuration := fixture(t)
	first := stageFixture(t, s, binary, configuration)
	again := stageFixture(t, s, binary, configuration)
	if first != again {
		t.Fatal("stage not idempotent")
	}
	backup, e := os.ReadFile(filepath.Join(s.directory, first.ID, "config.yaml"))
	if e != nil || string(backup) != testConfig {
		t.Fatal("configuration backup changed")
	}
	st, e := s.Select(0, first.ID)
	if e != nil || st.Generation != 1 || st.Previous != "" {
		t.Fatal(st, e)
	}
	same, e := s.Select(1, first.ID)
	if e != nil || same != st {
		t.Fatal("selection not idempotent")
	}
	writeFixture(t, binary, []byte("synthetic agent 2"))
	second := stageFixture(t, s, binary, configuration)
	st, e = s.Select(1, second.ID)
	if e != nil || st.Previous != first.ID || st.Generation != 2 {
		t.Fatal(st, e)
	}
	if _, e := s.Select(1, first.ID); !errors.Is(e, ErrConflict) {
		t.Fatal("stale generation accepted")
	}
	st, e = s.Rollback(2)
	if e != nil || st.Current != first.ID || st.Previous != second.ID || st.Generation != 3 {
		t.Fatal(st, e)
	}
	directory := s.directory
	if e := s.Close(); e != nil {
		t.Fatal(e)
	}
	if _, e := LoadSelected(directory, binary); e == nil {
		t.Fatal("wrong executable accepted")
	}
	writeFixture(t, binary, []byte("synthetic agent 1"))
	c, e := LoadSelected(directory, binary)
	if e != nil || c.HTTP.Listen != "127.0.0.1:18080" {
		t.Fatal("selected config did not load", e)
	}
}
func TestInvalidCandidatePreservesSelection(t *testing.T) {
	s, binary, configuration := fixture(t)
	m := stageFixture(t, s, binary, configuration)
	initial, e := s.Select(0, m.ID)
	if e != nil {
		t.Fatal(e)
	}
	if _, e := s.Stage(binary, strings.Repeat("0", 64), configuration); e == nil {
		t.Fatal("wrong hash accepted")
	}
	writeFixture(t, configuration, []byte(testConfig+"token: SENSITIVE-INVALID\n"))
	if _, e := s.Stage(binary, digest([]byte("synthetic agent 1")), configuration); e == nil || strings.Contains(e.Error(), "SENSITIVE") {
		t.Fatal("invalid config accepted or leaked")
	}
	if _, e := s.Select(1, "../../outside"); e == nil {
		t.Fatal("path traversal accepted")
	}
	writeFixture(t, filepath.Join(s.directory, m.ID, "agent"), []byte("corruption"))
	if _, e := s.Select(1, m.ID); e == nil {
		t.Fatal("corrupt binary accepted")
	}
	after, e := s.Status()
	if e != nil || after != initial {
		t.Fatal("selection changed on failure")
	}
}
func TestBackupCorruptionAndRollbackFailClosed(t *testing.T) {
	s, binary, configuration := fixture(t)
	first := stageFixture(t, s, binary, configuration)
	if _, e := s.Select(0, first.ID); e != nil {
		t.Fatal(e)
	}
	writeFixture(t, binary, []byte("second"))
	second := stageFixture(t, s, binary, configuration)
	before, e := s.Select(1, second.ID)
	if e != nil {
		t.Fatal(e)
	}
	writeFixture(t, filepath.Join(s.directory, first.ID, "config.yaml"), []byte(testConfig+"# tampered"))
	if _, e := s.Rollback(2); e == nil {
		t.Fatal("corrupt backup accepted")
	}
	after, e := s.Status()
	if e != nil || after != before {
		t.Fatal("failed rollback changed selection")
	}
	writeFixture(t, filepath.Join(s.directory, "selection.json"), []byte(`{"version":1,"generation":2,"current":"SENSITIVE","previous":""}`))
	if _, e := s.Status(); e == nil || strings.Contains(e.Error(), "SENSITIVE") {
		t.Fatal("corrupt selection accepted or leaked")
	}
}
func TestStorageLockQuotaAndConcurrentCAS(t *testing.T) {
	s, binary, configuration := fixture(t)
	if other, e := Open(s.directory); e == nil {
		other.Close()
		t.Fatal("concurrent writer accepted")
	}
	first := stageFixture(t, s, binary, configuration)
	writeFixture(t, binary, []byte("second"))
	second := stageFixture(t, s, binary, configuration)
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, id := range []string{first.ID, second.ID} {
		wg.Add(1)
		go func(id string) { defer wg.Done(); _, e := s.Select(0, id); results <- e }(id)
	}
	wg.Wait()
	close(results)
	success, conflicts := 0, 0
	for e := range results {
		if e == nil {
			success++
		} else if errors.Is(e, ErrConflict) {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if success != 1 || conflicts != 1 {
		t.Fatal("CAS lost update")
	}
	for i := 2; i < MaxReleases; i++ {
		writeFixture(t, binary, bytes.Repeat([]byte{byte(i)}, i))
		stageFixture(t, s, binary, configuration)
	}
	writeFixture(t, binary, []byte("over quota"))
	if _, e := s.Stage(binary, digest([]byte("over quota")), configuration); e == nil {
		t.Fatal("release quota ignored")
	}
	if _, e := s.Rollback(1); e == nil {
		t.Fatal("rollback without previous accepted")
	}
}
func TestStorageRejectsCheckoutAndSymlinks(t *testing.T) {
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	if s, e := Open(cwd); e == nil {
		s.Close()
		t.Fatal("checkout accepted")
	}
	s, binary, configuration := fixture(t)
	m := stageFixture(t, s, binary, configuration)
	path := filepath.Join(s.directory, m.ID, "manifest.json")
	writeFixture(t, path, []byte(`{"version":1,"extra":true}`))
	if _, _, e := s.verify(m.ID); e == nil {
		t.Fatal("invalid manifest accepted")
	}
	link := filepath.Join(t.TempDir(), "link")
	if e := os.Symlink(binary, link); e != nil {
		t.Skip("symlink creation unavailable on this platform")
	}
	if _, e := s.Stage(link, digest([]byte("synthetic agent 1")), configuration); e == nil {
		t.Fatal("symlink accepted")
	}
}

func TestOversizeInputsAndIncompleteReleaseFailClosed(t *testing.T) {
	s, binary, configuration := fixture(t)
	f, e := os.OpenFile(binary, os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	e = f.Truncate(MaxBinaryBytes + 1)
	f.Close()
	if e != nil {
		t.Fatal(e)
	}
	if _, e := s.Stage(binary, strings.Repeat("0", 64), configuration); e == nil {
		t.Fatal("oversize executable accepted")
	}
	writeFixture(t, binary, []byte("small"))
	writeFixture(t, configuration, bytes.Repeat([]byte("#"), 65537))
	if _, e := s.Stage(binary, digest([]byte("small")), configuration); e == nil {
		t.Fatal("oversize config accepted")
	}
	if _, e := s.Select(0, strings.Repeat("a", 64)); e == nil {
		t.Fatal("missing release accepted")
	}
	st, e := s.Status()
	if e != nil || st.Current != "" || st.Generation != 0 {
		t.Fatal("incomplete stage changed selection")
	}
}
