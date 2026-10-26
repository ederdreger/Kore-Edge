package provision

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func assignment(t *testing.T) Assignment {
	t.Helper()
	pub, _, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	return Assignment{EdgeID: "edge-lab", PartnerID: "partner-lab", CatalogPublicKey: base64.StdEncoding.EncodeToString(pub), TTLSeconds: 60}
}
func proof(t *testing.T, edge string) Request {
	t.Helper()
	key, e := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	public, e := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if e != nil {
		t.Fatal(e)
	}
	nonce, e := randomString(32)
	if e != nil {
		t.Fatal(e)
	}
	p := Proof{Version: 1, EdgeID: edge, PublicKey: base64.StdEncoding.EncodeToString(public), Challenge: nonce}
	payload, _ := json.Marshal(p)
	digest := sha256.Sum256(append([]byte(Domain), payload...))
	signature, e := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if e != nil {
		t.Fatal(e)
	}
	return Request{Proof: p, Signature: base64.StdEncoding.EncodeToString(signature)}
}
func setup(t *testing.T) (*Registry, Assignment, time.Time) {
	t.Helper()
	r, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	return r, assignment(t), time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
}

func TestSingleUseAndPersistentIdentity(t *testing.T) {
	r, a, now := setup(t)
	credential, e := r.Issue(context.Background(), a, now)
	if e != nil {
		t.Fatal(e)
	}
	request := proof(t, a.EdgeID)
	if strings.Contains(fmt.Sprintf("%+v %#v", credential, credential), credential.Token) {
		t.Fatal("credential debug formatting leaked bearer")
	}
	identity, e := r.Register(context.Background(), a.EdgeID, credential.Token, request, now.Add(time.Second))
	if e != nil {
		t.Fatal(e)
	}
	if identity.EdgeID != a.EdgeID || identity.PartnerID != a.PartnerID || identity.PublicKey != request.PublicKey || identity.Configuration.Catalog.PublicKey != a.CatalogPublicKey {
		t.Fatal("identity assignment changed")
	}
	if _, e = r.Register(context.Background(), a.EdgeID, credential.Token, request, now.Add(2*time.Second)); !errors.Is(e, ErrRejected) {
		t.Fatal("replayed credential")
	}
	reopened, e := Open(r.directory)
	if e != nil {
		t.Fatal(e)
	}
	status, e := reopened.Status(context.Background(), a.EdgeID)
	if e != nil || status.State != "registered" || status.Applied || status.Identity == nil {
		t.Fatal("identity lost across restart")
	}
	if _, e = reopened.Register(context.Background(), a.EdgeID, credential.Token, request, now.Add(3*time.Second)); e == nil {
		t.Fatal("consumption lost across restart")
	}
	if _, e = reopened.Issue(context.Background(), a, now.Add(4*time.Second)); e == nil {
		t.Fatal("silent replacement of registered identity")
	}
	bytes, e := os.ReadFile(filepath.Join(r.directory, "registry.json"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(bytes), credential.Token) || strings.Contains(string(bytes), request.Signature) {
		t.Fatal("raw credentials/proof leaked to registry")
	}
}

func TestExpirationRevocationAndNotBefore(t *testing.T) {
	for _, test := range []string{"expires-at-boundary", "expired", "revoked", "not-before"} {
		t.Run(test, func(t *testing.T) {
			r, a, now := setup(t)
			credential, e := r.Issue(context.Background(), a, now)
			if e != nil {
				t.Fatal(e)
			}
			when := now.Add(time.Second)
			switch test {
			case "expires-at-boundary":
				when = credential.ExpiresAt
			case "expired":
				when = credential.ExpiresAt.Add(time.Second)
			case "not-before":
				when = now.Add(-time.Second)
			case "revoked":
				if r.Revoke(context.Background(), credential.ID, now.Add(time.Second)) != nil || r.Revoke(context.Background(), credential.ID, now.Add(2*time.Second)) != nil {
					t.Fatal("idempotent revocation")
				}
				var err error
				r, err = Open(r.directory)
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, e = r.Register(context.Background(), a.EdgeID, credential.Token, proof(t, a.EdgeID), when); !errors.Is(e, ErrRejected) {
				t.Fatal("invalid credential accepted")
			}
			status, e := r.Status(context.Background(), a.EdgeID)
			if e != nil || status.Identity != nil {
				t.Fatal("rejected request mutated identity")
			}
		})
	}
}

func TestProofAndBindingFailuresDoNotConsume(t *testing.T) {
	for _, kind := range []string{"signature", "challenge", "edge", "public-key", "version", "unknown-token"} {
		t.Run(kind, func(t *testing.T) {
			r, a, now := setup(t)
			credential, e := r.Issue(context.Background(), a, now)
			if e != nil {
				t.Fatal(e)
			}
			valid := proof(t, a.EdgeID)
			bad := valid
			token := credential.Token
			switch kind {
			case "signature":
				bad.Signature = base64.StdEncoding.EncodeToString(make([]byte, 64))
			case "challenge":
				bad.Challenge = "invalid"
			case "edge":
				bad.EdgeID = "other-edge"
			case "public-key":
				bad.PublicKey = a.CatalogPublicKey
			case "version":
				bad.Version = 2
			case "unknown-token":
				suffix, _ := randomString(32)
				token = "ke1_" + suffix
			}
			if _, e = r.Register(context.Background(), a.EdgeID, token, bad, now.Add(time.Second)); e == nil {
				t.Fatal("bad proof/binding accepted")
			}
			if _, e = r.Register(context.Background(), a.EdgeID, credential.Token, valid, now.Add(2*time.Second)); e != nil {
				t.Fatal("bad proof consumed token")
			}
		})
	}
}

func TestConcurrentConsumptionAcrossRegistryInstances(t *testing.T) {
	r, a, now := setup(t)
	credential, e := r.Issue(context.Background(), a, now)
	if e != nil {
		t.Fatal(e)
	}
	other, e := Open(r.directory)
	if e != nil {
		t.Fatal(e)
	}
	requests := make([]Request, 16)
	for i := range requests {
		requests[i] = proof(t, a.EdgeID)
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i, request := range requests {
		wg.Add(1)
		go func(i int, request Request) {
			defer wg.Done()
			registry := r
			if i%2 == 1 {
				registry = other
			}
			if _, e := registry.Register(context.Background(), a.EdgeID, credential.Token, request, now.Add(time.Second)); e == nil {
				accepted.Add(1)
			} else if !errors.Is(e, ErrRejected) && !errors.Is(e, ErrBusy) {
				t.Error("unexpected enrollment failure")
			}
		}(i, request)
	}
	wg.Wait()
	if accepted.Load() != 1 {
		t.Fatal("token was not consumed exactly once")
	}
}

func TestPersistenceFailureRetainsPendingToken(t *testing.T) {
	r, a, now := setup(t)
	credential, e := r.Issue(context.Background(), a, now)
	if e != nil {
		t.Fatal(e)
	}
	request := proof(t, a.EdgeID)
	r.save = func(database) error { return ErrStorage }
	if _, e = r.Register(context.Background(), a.EdgeID, credential.Token, request, now.Add(time.Second)); !errors.Is(e, ErrStorage) {
		t.Fatal("failed commit reported success")
	}
	r.save = r.persist
	status, e := r.Status(context.Background(), a.EdgeID)
	if e != nil || status.Identity != nil {
		t.Fatal("partial commit")
	}
	if _, e = r.Register(context.Background(), a.EdgeID, credential.Token, request, now.Add(2*time.Second)); e != nil {
		t.Fatal("failed commit consumed token")
	}
}
func TestMalformedStateAndBusyFailClosed(t *testing.T) {
	r, _, _ := setup(t)
	file := filepath.Join(r.directory, "registry.json")
	if os.WriteFile(file, []byte(`{"version":1,"version":1}`), 0600) != nil {
		t.Fatal("fixture")
	}
	if _, e := r.Status(context.Background(), "edge-lab"); !errors.Is(e, ErrStorage) {
		t.Fatal("corrupt state ignored")
	}
	os.Remove(file)
	lock := filepath.Join(r.directory, "registry.lock")
	if os.WriteFile(lock, nil, 0600) != nil {
		t.Fatal("lock fixture")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, e := r.Status(ctx, "edge-lab"); !errors.Is(e, ErrBusy) {
		t.Fatal("stale/busy lock bypassed")
	}
	if _, e := os.Stat(lock); e != nil {
		t.Fatal("foreign lock removed")
	}
	os.Remove(lock)
}
func TestConfigurationAndRuntimePaths(t *testing.T) {
	cwd, e := os.Getwd()
	if e != nil {
		t.Fatal(e)
	}
	if _, e = Open(filepath.Join(cwd, "runtime")); !errors.Is(e, ErrStorage) {
		t.Fatal("checkout path allowed")
	}
	if (Config{}).Validate() != nil {
		t.Fatal("disabled config")
	}
	if (Config{Enabled: true, EdgeID: "edge", Directory: "relative"}).Validate() == nil {
		t.Fatal("relative path")
	}
	_, a, now := setup(t)
	r, e := Open(t.TempDir())
	if e != nil {
		t.Fatal(e)
	}
	a.TTLSeconds = 901
	if _, e = r.Issue(context.Background(), a, now); e == nil {
		t.Fatal("unbounded TTL")
	}
}

func TestRevokingConsumedBootstrapDoesNotDeprovisionIdentity(t *testing.T) {
	r, a, now := setup(t)
	credential, e := r.Issue(context.Background(), a, now)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.Register(context.Background(), a.EdgeID, credential.Token, proof(t, a.EdgeID), now.Add(time.Second)); e != nil {
		t.Fatal(e)
	}
	if r.Revoke(context.Background(), credential.ID, now.Add(2*time.Second)) != nil {
		t.Fatal("revoke")
	}
	status, e := r.Status(context.Background(), a.EdgeID)
	if e != nil || status.Identity == nil || status.State != "registered" {
		t.Fatal("bootstrap revocation modified identity")
	}
}
