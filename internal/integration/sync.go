// Package integration implements the proposed Kore-Player TV catalog contract.
// This phase supports only an in-process mock transport, never a remote host.
package integration

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"regexp"
	"strconv"
	"sync"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/security"
)

var ErrRejected = errors.New("integration configuration rejected")

const MediaType = "application/vnd.kore-edge.catalog.v1+json"

type Config struct {
	Enabled         bool   `yaml:"enabled"`
	Mode            string `yaml:"mode"`
	EdgeID          string `yaml:"edge_id"`
	MockCatalog     string `yaml:"mock_catalog"`
	IntervalSeconds int    `yaml:"interval_seconds"`
}

var identifier = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

func (c Config) Validate() error {
	if !c.Enabled {
		if c.Mode != "" || c.EdgeID != "" || c.MockCatalog != "" || c.IntervalSeconds != 0 {
			return ErrRejected
		}
		return nil
	}
	if c.Mode != "mock" || !identifier.MatchString(c.EdgeID) || !filepath.IsAbs(c.MockCatalog) || c.IntervalSeconds < 1 || c.IntervalSeconds > 3600 {
		return ErrRejected
	}
	return nil
}

type Status struct {
	Mode                string     `json:"mode"`
	State               string     `json:"state"`
	Attempts            int        `json:"attempts"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	LastAttempt         *time.Time `json:"last_attempt"`
	LastSuccess         *time.Time `json:"last_success"`
	NextAttempt         *time.Time `json:"next_attempt"`
	CatalogAvailable    bool       `json:"catalog_available"`
	Revision            uint64     `json:"revision"`
}
type Service struct {
	mu        sync.Mutex
	config    Config
	store     *catalog.Store
	transport http.RoundTripper
	etag      string
	status    Status
	running   bool
}

// NewMock does not open a socket. It reads the signed mock response from a
// private runtime file for each request, allowing deterministic fault injection.
func NewMock(c Config, store *catalog.Store) (*Service, error) {
	if !c.Enabled || c.Validate() != nil || store == nil {
		return nil, ErrRejected
	}
	return &Service{config: c, store: store, transport: mockTransport{c.MockCatalog, c.EdgeID}, status: Status{Mode: "mock", State: "waiting"}}, nil
}
func (s *Service) Snapshot(now time.Time) Status {
	s.mu.Lock()
	defer s.mu.Unlock()
	st := s.status
	clone := func(p *time.Time) *time.Time {
		if p == nil {
			return nil
		}
		v := *p
		return &v
	}
	st.LastAttempt = clone(st.LastAttempt)
	st.LastSuccess = clone(st.LastSuccess)
	st.NextAttempt = clone(st.NextAttempt)
	m, e := s.store.Snapshot(now)
	st.CatalogAvailable = e == nil
	if e == nil {
		st.Revision = m.Revision
	}
	return st
}

// Run performs at most three consecutive failed attempts, then remains stopped
// until an explicit agent restart. Successful polls use the configured interval.
func (s *Service) Run(ctx context.Context) {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return
	}
	s.running = true
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		s.status.NextAttempt = nil
		if s.status.State != "exhausted" {
			s.status.State = "stopped"
		}
		s.mu.Unlock()
	}()
	failures := 0
	for {
		if ctx.Err() != nil {
			return
		}
		ok := s.poll(ctx, time.Now().UTC())
		if ctx.Err() != nil {
			return
		}
		if ok {
			failures = 0
		} else {
			failures++
		}
		delay := time.Duration(s.config.IntervalSeconds) * time.Second
		s.mu.Lock()
		s.status.ConsecutiveFailures = failures
		if failures >= 3 {
			s.status.State = "exhausted"
			s.mu.Unlock()
			return
		}
		if failures > 0 {
			delay = time.Second * time.Duration(1<<uint(failures-1))
			s.status.State = "backoff"
		}
		next := time.Now().UTC().Add(delay)
		s.status.NextAttempt = &next
		s.mu.Unlock()
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
func (s *Service) poll(ctx context.Context, now time.Time) bool {
	s.mu.Lock()
	s.status.Attempts++
	s.status.LastAttempt = &now
	s.status.NextAttempt = nil
	s.status.State = "syncing"
	etag := s.etag
	s.mu.Unlock()
	requestCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(requestCtx, http.MethodGet, "https://mock.invalid/api/v1/edges/"+s.config.EdgeID+"/catalog", nil)
	req.Header.Set("Accept", MediaType)
	if etag != "" {
		req.Header.Set("If-None-Match", etag)
	}
	resp, e := s.transport.RoundTrip(req)
	fail := func() bool { s.mu.Lock(); s.status.State = "unavailable"; s.mu.Unlock(); return false }
	if e != nil {
		return fail()
	}
	if resp == nil || resp.Body == nil {
		return fail()
	}
	defer resp.Body.Close()
	if requestCtx.Err() != nil {
		return fail()
	}
	if resp.StatusCode == http.StatusNotModified {
		if etag == "" || resp.Header.Get("ETag") != etag {
			return fail()
		}
		if _, e := s.store.Snapshot(now); e != nil {
			return fail()
		}
	} else {
		if resp.StatusCode != http.StatusOK || resp.Header.Get("Content-Type") != MediaType || resp.Header.Get("Content-Encoding") != "" || resp.ContentLength > catalog.MaxBytes {
			return fail()
		}
		b, e := io.ReadAll(io.LimitReader(resp.Body, catalog.MaxBytes+1))
		if e != nil || len(b) > catalog.MaxBytes || requestCtx.Err() != nil {
			return fail()
		}
		if s.store.Apply(b, time.Now().UTC()) != nil {
			return fail()
		}
		sum := sha256.Sum256(b)
		etag = `"` + hex.EncodeToString(sum[:]) + `"`
		// A server validator is accepted only when bound to the exact verified bytes.
		if resp.Header.Get("ETag") != etag {
			etag = ""
		}
	}
	stamp := time.Now().UTC()
	s.mu.Lock()
	s.etag = etag
	s.status.State = "synchronized"
	s.status.LastSuccess = &stamp
	s.mu.Unlock()
	return true
}

type mockTransport struct{ path, edgeID string }

func (t mockTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	if r.Context().Err() != nil {
		return nil, ErrRejected
	}
	if r.Method != "GET" || r.URL.Scheme != "https" || r.URL.Host != "mock.invalid" || r.URL.Path != "/api/v1/edges/"+t.edgeID+"/catalog" || r.URL.RawQuery != "" || r.Header.Get("Accept") != MediaType {
		return nil, ErrRejected
	}
	b, e := security.ReadPrivateFile(t.path, catalog.MaxBytes)
	if e != nil {
		return &http.Response{StatusCode: 503, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(nil)), Request: r}, nil
	}
	sum := sha256.Sum256(b)
	etag := `"` + hex.EncodeToString(sum[:]) + `"`
	header := make(http.Header)
	header.Set("ETag", etag)
	header.Set("Content-Type", MediaType)
	header.Set("Cache-Control", "no-store")
	if r.Header.Get("If-None-Match") == etag {
		return &http.Response{StatusCode: 304, Header: header, Body: io.NopCloser(bytes.NewReader(nil)), Request: r}, nil
	}
	header.Set("Content-Length", strconv.Itoa(len(b)))
	return &http.Response{StatusCode: 200, Header: header, ContentLength: int64(len(b)), Body: io.NopCloser(bytes.NewReader(b)), Request: r}, nil
}
