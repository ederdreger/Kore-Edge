package catalog

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"
)

func setup(t *testing.T) (*Store, ed25519.PrivateKey, Manifest, time.Time) {
	t.Helper()
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	s, e := New("partner-lab", base64.StdEncoding.EncodeToString(pub))
	if e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile("../../configs/catalog-payload.example.json")
	if e != nil {
		t.Fatal(e)
	}
	var m Manifest
	if json.Unmarshal(b, &m) != nil {
		t.Fatal("example")
	}
	return s, key, m, m.IssuedAt.Add(time.Hour)
}
func signed(t *testing.T, key ed25519.PrivateKey, m Manifest) []byte {
	t.Helper()
	p, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	return signedPayload(t, key, p)
}
func signedPayload(t *testing.T, key ed25519.PrivateKey, p []byte) []byte {
	t.Helper()
	signature := ed25519.Sign(key, append([]byte(Domain), p...))
	b, e := json.Marshal(Envelope{base64.StdEncoding.EncodeToString(p), base64.StdEncoding.EncodeToString(signature)})
	if e != nil {
		t.Fatal(e)
	}
	return b
}

func TestValidAndAuthorization(t *testing.T) {
	s, key, m, now := setup(t)
	body := signed(t, key, m)
	if e := s.Apply(body, now); e != nil {
		t.Fatal(e)
	}
	if e := s.Apply(body, now); e != nil {
		t.Fatal("idempotency")
	}
	if !s.Authorize("test-pattern", "udp", now) || s.Authorize("other", "udp", now) || s.Authorize("test-pattern", "other", now) {
		t.Fatal("permissions")
	}
	got, e := s.Snapshot(now)
	if e != nil {
		t.Fatal(e)
	}
	got.Channels[0].Outputs[0] = "other"
	if !s.Authorize("test-pattern", "udp", now) {
		t.Fatal("mutable snapshot")
	}
	if s.Authorize("test-pattern", "udp", m.ExpiresAt) {
		t.Fatal("expired permission")
	}
}

func TestRejectedUpdatesRetainSnapshot(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Manifest)
	}{
		{"partner", func(m *Manifest) { m.PartnerID = "another" }},
		{"schema", func(m *Manifest) { m.Version = 2 }},
		{"zero revision", func(m *Manifest) { m.Revision = 0 }},
		{"expired", func(m *Manifest) { m.ExpiresAt = m.IssuedAt }},
		{"future", func(m *Manifest) { m.IssuedAt = m.IssuedAt.Add(2 * time.Hour) }},
		{"long validity", func(m *Manifest) { m.ExpiresAt = m.ExpiresAt.Add(time.Hour) }},
		{"unsigned channel permission", func(m *Manifest) { m.Grants = nil }},
		{"unauthorized output", func(m *Manifest) { m.Grants[0].Outputs = []string{"hls"} }},
		{"orphan grant", func(m *Manifest) { m.Grants[0].ChannelID = "missing" }},
		{"duplicate channel", func(m *Manifest) { m.Channels = append(m.Channels, m.Channels[0]) }},
		{"duplicate grant", func(m *Manifest) { m.Grants = append(m.Grants, m.Grants[0]) }},
		{"real source", func(m *Manifest) { m.Channels[0].Source = "http://example.invalid/channel" }},
		{"invalid output", func(m *Manifest) { m.Channels[0].Outputs = []string{"unknown"} }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, key, m, now := setup(t)
			if s.Apply(signed(t, key, m), now) != nil {
				t.Fatal("baseline")
			}
			tc.change(&m)
			m.Revision = 2
			if tc.name == "zero revision" {
				m.Revision = 0
			}
			if s.Apply(signed(t, key, m), now) == nil {
				t.Fatal("accepted invalid update")
			}
			got, e := s.Snapshot(now)
			if e != nil || got.Revision != 1 {
				t.Fatal("lost baseline")
			}
		})
	}
}

func TestSignatureAndReplay(t *testing.T) {
	s, key, m, now := setup(t)
	if s.Apply(signed(t, key, m), now) != nil {
		t.Fatal("baseline")
	}
	_, wrong, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	m.Revision = 2
	if s.Apply(signed(t, wrong, m), now) == nil {
		t.Fatal("wrong signer")
	}
	if s.Apply(signed(t, key, m), now) != nil {
		t.Fatal("new revision")
	}
	m.Revision = 1
	if s.Apply(signed(t, key, m), now) == nil {
		t.Fatal("downgrade")
	}
	m.Revision = 2
	m.Channels[0].Name = "changed"
	if s.Apply(signed(t, key, m), now) == nil {
		t.Fatal("equivocation")
	}
	raw, _ := json.Marshal(m)
	raw = append(raw[:len(raw)-1], []byte(`,"revision":3}`)...)
	if s.Apply(signedPayload(t, key, raw), now) == nil {
		t.Fatal("duplicate keys")
	}
	if s.Apply([]byte(`{"payload":"bad","signature":"bad"}`), now) == nil {
		t.Fatal("bad envelope")
	}
}

func TestStrictJSON(t *testing.T) {
	for _, b := range []string{`{"version":1,"version":2}`, `{"unknown":1}`, `{} {}`, `null []`} {
		var m Manifest
		if DecodeStrict([]byte(b), &m) == nil {
			t.Fatal("ambiguous JSON accepted")
		}
	}
	var v any
	if DecodeStrict(make([]byte, MaxBytes+1), &v) == nil {
		t.Fatal("oversize")
	}
}

func TestConcurrentSnapshots(t *testing.T) {
	s, key, m, now := setup(t)
	body := signed(t, key, m)
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if s.Apply(body, now) != nil {
				t.Error("apply")
			}
			if !s.Authorize("test-pattern", "udp", now) {
				t.Error("authorize")
			}
		}()
	}
	wg.Wait()
}

func TestTamperingAndRevocation(t *testing.T) {
	s, key, m, now := setup(t)
	body := signed(t, key, m)
	var env Envelope
	if json.Unmarshal(body, &env) != nil {
		t.Fatal("envelope")
	}
	payload, _ := base64.StdEncoding.DecodeString(env.Payload)
	payload = append(payload, ' ')
	env.Payload = base64.StdEncoding.EncodeToString(payload)
	tampered, _ := json.Marshal(env)
	if s.Apply(tampered, now) == nil {
		t.Fatal("tampered bytes accepted")
	}
	if s.Apply(body, now) != nil {
		t.Fatal("baseline")
	}
	m.Revision = 2
	m.Channels = []Channel{}
	m.Grants = []Grant{}
	if s.Apply(signed(t, key, m), now) != nil || s.Authorize("test-pattern", "udp", now) {
		t.Fatal("revocation failed")
	}
	// Restore earlier content only with a newly signed higher revision.
	var original Manifest
	payloadOriginal, _ := base64.StdEncoding.DecodeString(func() string {
		var originalEnv Envelope
		_ = json.Unmarshal(body, &originalEnv)
		return originalEnv.Payload
	}())
	if json.Unmarshal(payloadOriginal, &original) != nil {
		t.Fatal("original payload")
	}
	original.Revision = 3
	if s.Apply(signed(t, key, original), now) != nil || !s.Authorize("test-pattern", "udp", now) {
		t.Fatal("logical rollback failed")
	}
}
