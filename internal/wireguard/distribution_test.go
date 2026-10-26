package wireguard

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestPinnedDistributionIdentityInvalidatesOnPeerRevisionAndClose(t *testing.T) {
	store, e := OpenCredentials(filepath.Join(t.TempDir(), "wg"))
	if e != nil {
		t.Fatal(e)
	}
	defer store.Close()
	identity, e := store.EnsureIdentity()
	if e != nil {
		t.Fatal(e)
	}
	first := syntheticSettings(t)
	saved, e := store.SavePeer(first, identity.Revision)
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 3; i++ {
		if !store.MatchesDistribution(identity.PublicKey, first.KorePlayPublicKey) {
			t.Fatal("pinned identity rejected")
		}
	}
	secret, e := store.SRTSecret(strings.Repeat("a", 32))
	if e != nil || len(secret) != 32 {
		t.Fatal("SRT derivation failed")
	}
	other, e := store.SRTSecret(strings.Repeat("b", 32))
	if e != nil || secret == other {
		t.Fatal("partner secret isolation")
	}
	if _, e = store.SRTSecret("../invalid"); e == nil {
		t.Fatal("invalid partner accepted")
	}
	second := syntheticSettings(t)
	if _, e = store.SavePeer(second, saved.Revision); e != nil {
		t.Fatal(e)
	}
	if store.MatchesDistribution(identity.PublicKey, first.KorePlayPublicKey) || !store.MatchesDistribution(identity.PublicKey, second.KorePlayPublicKey) {
		t.Fatal("stale pinned key accepted")
	}
	store.Close()
	if store.MatchesDistribution(identity.PublicKey, second.KorePlayPublicKey) {
		t.Fatal("closed identity accepted")
	}
}
