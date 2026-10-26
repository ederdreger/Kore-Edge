package integration

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/stream"
)

type fixture struct {
	service *Service
	key     ed25519.PrivateKey
	path    string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	public, private, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	store, e := catalog.New("partner-lab", base64.StdEncoding.EncodeToString(public))
	if e != nil {
		t.Fatal(e)
	}
	dir := t.TempDir()
	if e := os.Chmod(dir, 0700); e != nil {
		t.Fatal(e)
	}
	path := filepath.Join(dir, "catalog.json")
	service, e := NewMock(Config{Enabled: true, Mode: "mock", EdgeID: "edge-lab", MockCatalog: path, IntervalSeconds: 1}, store)
	if e != nil {
		t.Fatal(e)
	}
	return &fixture{service, private, path}
}
func (f *fixture) envelope(t *testing.T, revision uint64, partner, name string, issued, expires time.Time) []byte {
	t.Helper()
	m := catalog.Manifest{Version: 1, Revision: revision, PartnerID: partner, IssuedAt: issued, ExpiresAt: expires, Channels: []catalog.Channel{{ID: "test", Name: name, Source: "synthetic://test", Outputs: []string{"udp"}}}, Grants: []catalog.Grant{{ChannelID: "test", Outputs: []string{"udp"}}}}
	payload, e := json.Marshal(m)
	if e != nil {
		t.Fatal(e)
	}
	sig := ed25519.Sign(f.key, append([]byte(catalog.Domain), payload...))
	b, e := json.Marshal(catalog.Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(sig)})
	if e != nil {
		t.Fatal(e)
	}
	return b
}
func (f *fixture) write(t *testing.T, b []byte) {
	t.Helper()
	if e := os.WriteFile(f.path, b, 0600); e != nil {
		t.Fatal(e)
	}
}
func (f *fixture) valid(t *testing.T, revision uint64) []byte {
	t.Helper()
	now := time.Now().UTC()
	return f.envelope(t, revision, "partner-lab", "Synthetic test", now.Add(-time.Minute), now.Add(time.Hour))
}

func TestSignedSynchronizationAndConditionalRequest(t *testing.T) {
	f := newFixture(t)
	f.write(t, f.valid(t, 1))
	s := f.service
	if !s.poll(context.Background(), time.Now().UTC()) {
		t.Fatal("initial sync failed")
	}
	first := s.Snapshot(time.Now().UTC())
	if first.Mode != "mock" || first.State != "synchronized" || first.Revision != 1 || !first.CatalogAvailable || s.etag == "" {
		t.Fatal(first)
	}
	// Same signed bytes produce HTTP 304, retaining the validated catalog.
	if !s.poll(context.Background(), time.Now().UTC()) || s.Snapshot(time.Now().UTC()).Revision != 1 {
		t.Fatal("conditional sync failed")
	}
	f.write(t, f.valid(t, 2))
	if !s.poll(context.Background(), time.Now().UTC()) || s.Snapshot(time.Now().UTC()).Revision != 2 {
		t.Fatal("revision update failed")
	}
	*first.LastSuccess = time.Time{}
	if s.Snapshot(time.Now().UTC()).LastSuccess.IsZero() {
		t.Fatal("mutable snapshot")
	}
	future := time.Now().UTC().Add(2 * time.Hour)
	if s.poll(context.Background(), future) || s.Snapshot(future).CatalogAvailable {
		t.Fatal("304 extended signed validity")
	}
}

func TestRejectedUpdatesPreserveLastValidCatalog(t *testing.T) {
	f := newFixture(t)
	s := f.service
	f.write(t, f.valid(t, 2))
	if !s.poll(context.Background(), time.Now().UTC()) {
		t.Fatal("baseline")
	}
	now := time.Now().UTC()
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"replay", f.valid(t, 1)},
		{"same revision different payload", f.envelope(t, 2, "partner-lab", "changed", now.Add(-time.Minute), now.Add(time.Hour))},
		{"other partner", f.envelope(t, 3, "other-partner", "SENSITIVE", now.Add(-time.Minute), now.Add(time.Hour))},
		{"expired", f.envelope(t, 3, "partner-lab", "test", now.Add(-time.Hour), now.Add(-time.Minute))},
		{"future", f.envelope(t, 3, "partner-lab", "test", now.Add(time.Minute), now.Add(time.Hour))},
		{"invalid signature", []byte(`{"payload":"SENSITIVE","signature":"SENSITIVE"}`)},
		{"duplicate JSON", []byte(`{"payload":"x","payload":"y","signature":"z"}`)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f.write(t, tc.body)
			if s.poll(context.Background(), time.Now().UTC()) {
				t.Fatal("invalid update accepted")
			}
			st := s.Snapshot(time.Now().UTC())
			if st.Revision != 2 || !st.CatalogAvailable || !s.store.Authorize("test", "udp", time.Now().UTC()) {
				t.Fatal("invalid response destroyed valid state")
			}
			b, _ := json.Marshal(st)
			if strings.Contains(string(b), "SENSITIVE") || strings.Contains(string(b), f.path) || strings.Contains(string(b), "partner-lab") {
				t.Fatal("sensitive status")
			}
		})
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestHTTPContractRejectsUnexpectedResponses(t *testing.T) {
	for _, tc := range []struct {
		name            string
		status          int
		media, encoding string
		body            []byte
	}{
		{"redirect", 302, MediaType, "", nil}, {"unauthorized", 401, MediaType, "", nil}, {"failure", 500, MediaType, "", nil},
		{"wrong content type", 200, "application/json", "", nil}, {"compressed", 200, MediaType, "gzip", nil},
		{"oversize", 200, MediaType, "", bytes.Repeat([]byte("x"), catalog.MaxBytes+1)},
		{"304 without baseline", 304, MediaType, "", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t)
			calls := 0
			f.service.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
				calls++
				if r.URL.Host != "mock.invalid" || r.URL.Path != "/api/v1/edges/edge-lab/catalog" || r.Method != "GET" || r.Header.Get("Accept") != MediaType {
					t.Error("request contract")
				}
				return &http.Response{StatusCode: tc.status, Header: http.Header{"Content-Type": []string{tc.media}, "Content-Encoding": []string{tc.encoding}, "Location": []string{"https://production.invalid/"}}, Body: io.NopCloser(bytes.NewReader(tc.body))}, nil
			})
			if f.service.poll(context.Background(), time.Now().UTC()) || calls != 1 {
				t.Fatal("unexpected response accepted or redirected")
			}
			if f.service.Snapshot(time.Now().UTC()).CatalogAvailable {
				t.Fatal("invented catalog")
			}
		})
	}
}

func TestMissingMockExhaustsBudgetWithoutInterruptingStream(t *testing.T) {
	f := newFixture(t)
	if e := f.service.store.Apply(f.valid(t, 1), time.Now().UTC()); e != nil {
		t.Fatal(e)
	}
	engine, e := stream.New(context.Background(), func(ctx context.Context) (*stream.Process, error) {
		child, cancel := context.WithCancel(ctx)
		r, w := io.Pipe()
		done := make(chan error, 1)
		go func() { err := stream.Generate(child, w); w.Close(); done <- err }()
		return &stream.Process{Output: r, Kill: func() error { cancel(); return w.Close() }, Wait: func() error { return <-done }}, nil
	}, func(id string, now time.Time) bool { return f.service.store.Authorize(id, "udp", now) }, stream.DefaultPolicy())
	if e != nil {
		t.Fatal(e)
	}
	defer engine.Close()
	if engine.Start("test") != nil {
		t.Fatal("start")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	f.service.Run(ctx)
	st := f.service.Snapshot(time.Now().UTC())
	if st.State != "exhausted" || st.Attempts != 3 || st.ConsecutiveFailures != 3 || !st.CatalogAvailable || st.NextAttempt != nil {
		t.Fatal(st)
	}
	status := engine.Snapshot()
	if len(status) != 1 || status[0].State != "running" || status[0].Attempts != 1 || status[0].Packets < 7 {
		t.Fatal("mock outage interrupted healthy data plane", status)
	}
}

func TestCancellationAndSingleWorker(t *testing.T) {
	f := newFixture(t)
	started := make(chan struct{}, 1)
	f.service.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		started <- struct{}{}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { f.service.Run(ctx); close(done) }()
	<-started
	duplicate := make(chan struct{})
	go func() { f.service.Run(ctx); close(duplicate) }()
	select {
	case <-duplicate:
	case <-time.After(time.Second):
		t.Fatal("duplicate worker")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancellation ignored")
	}
	if f.service.Snapshot(time.Now().UTC()).State != "stopped" {
		t.Fatal("wrong terminal state")
	}
}

func TestConfigurationAllowsOnlyExplicitMock(t *testing.T) {
	for _, c := range []Config{{Enabled: true, Mode: "remote"}, {Enabled: true, Mode: "mock", EdgeID: "bad/path", MockCatalog: "/private/file", IntervalSeconds: 1}, {Enabled: true, Mode: "mock", EdgeID: "edge", MockCatalog: "relative", IntervalSeconds: 1}, {Mode: "mock"}} {
		if c.Validate() == nil {
			t.Fatal("unsafe config accepted")
		}
	}
	if (Config{}).Validate() != nil {
		t.Fatal("default disabled rejected")
	}
}

func TestRequestDeadlineAndUntrustedETag(t *testing.T) {
	f := newFixture(t)
	f.service.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 2*time.Second {
			t.Error("missing request deadline")
		}
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	if f.service.poll(context.Background(), time.Now().UTC()) {
		t.Fatal("timed out request accepted")
	}
	body := f.valid(t, 1)
	f.service.transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{MediaType}, "Etag": []string{`"SENSITIVE-UNTRUSTED"`}}, Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	if !f.service.poll(context.Background(), time.Now().UTC()) || f.service.etag != "" {
		t.Fatal("unverified validator retained")
	}
}
