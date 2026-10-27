package playback

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestScopeTamperingExpiryAndRestart(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "playback")
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	owner := strings.Repeat("a", 32)
	raw, expires, e := s.Issue("synthetic", owner, 1, false)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Verify(raw, "synthetic", false); e != nil {
		t.Fatal(e)
	}
	for _, tc := range []struct {
		raw, id string
		hls     bool
	}{{raw, "other", false}, {raw, "synthetic", true}, {"x" + raw, "synthetic", false}, {raw + "x", "synthetic", false}} {
		if _, e = s.Verify(tc.raw, tc.id, tc.hls); e == nil {
			t.Fatal("capability expanded")
		}
	}
	restarted, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = restarted.Verify(raw, "synthetic", false); e != nil {
		t.Fatal("restart invalidated link", e)
	}
	data, e := os.ReadFile(filepath.Join(dir, "playback.json"))
	if e != nil || strings.Contains(string(data), raw) {
		t.Fatal("raw playback URL persisted")
	}
	s.now = func() time.Time { return expires }
	if _, e = s.Verify(raw, "synthetic", false); e == nil {
		t.Fatal("expired capability accepted")
	}
}

func TestRevocationPersistsAndConcurrencyIsBounded(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "playback")
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	raw, _, e := s.Issue("synthetic", strings.Repeat("a", 32), 1, true)
	if e != nil {
		t.Fatal(e)
	}
	a, ok := s.Acquire(raw)
	if !ok {
		t.Fatal("first consumer")
	}
	defer a()
	b, ok := s.Acquire(raw)
	if !ok {
		t.Fatal("second consumer")
	}
	defer b()
	if _, ok = s.Acquire(raw); ok {
		t.Fatal("unbounded capability consumers")
	}
	a()
	a()
	if e = s.Revoke([]string{"synthetic"}); e != nil {
		t.Fatal(e)
	}
	restarted, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = restarted.Verify(raw, "synthetic", false); e == nil {
		t.Fatal("revoked capability survived restart")
	}
	next, _, e := restarted.Issue("synthetic", strings.Repeat("a", 32), 1, true)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = restarted.Verify(next, "synthetic", true); e != nil {
		t.Fatal(e)
	}
}
