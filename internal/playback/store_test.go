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

func TestPermanentChannelAddressSurvivesClockAndRestartUntilDeletion(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "playback")
	s, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.Permanent("synthetic")
	if e != nil {
		t.Fatal(e)
	}
	s.now = func() time.Time { return time.Now().AddDate(30, 0, 0) }
	for i := 0; i < 3; i++ {
		again, e := s.Permanent("synthetic")
		if e != nil || again != first {
			t.Fatal("permanent link rotated")
		}
	}
	claims, e := s.Verify(first, "synthetic", true)
	if e != nil || !claims.Permanent || claims.Owner != "" || claims.Expires != 0 {
		t.Fatal("link depends on time or administrator", e)
	}
	if _, e = s.Verify(first, "other", false); e == nil {
		t.Fatal("channel scope escaped")
	}
	next, e := Open(dir)
	if e != nil {
		t.Fatal(e)
	}
	again, e := next.Permanent("synthetic")
	if e != nil || again != first {
		t.Fatal("restart changed address")
	}
	if next.Revoke([]string{"synthetic"}) != nil {
		t.Fatal("delete invalidation")
	}
	if _, e = next.Verify(first, "synthetic", false); e == nil {
		t.Fatal("deleted channel link accepted")
	}
	recreated, e := next.Permanent("synthetic")
	if e != nil || recreated == first {
		t.Fatal("recreated channel reused deleted link")
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
