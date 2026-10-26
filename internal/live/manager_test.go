package live

import (
	"context"
	"io"
	"kore-edge/internal/edgecatalog"
	"kore-edge/internal/multicast"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type syntheticCatalog struct {
	allowed atomic.Bool
	opens   atomic.Int32
}

func (c *syntheticCatalog) OpenSRT(context.Context, string) (io.ReadCloser, error) {
	return nil, ErrDenied
}

func (c *syntheticCatalog) Snapshot() edgecatalog.Snapshot {
	return edgecatalog.Snapshot{Available: c.allowed.Load()}
}
func (c *syntheticCatalog) Authorized(id, protocol string) bool {
	return c.allowed.Load() && safeID.MatchString(id) && protocol == "http_ts"
}
func (c *syntheticCatalog) OutputAuthorized(string, string) bool { return false }
func (c *syntheticCatalog) OpenHLS(context.Context, string, string) (io.ReadCloser, error) {
	return nil, ErrDenied
}
func (c *syntheticCatalog) OpenHTTP(ctx context.Context, id string) (io.ReadCloser, error) {
	if !c.Authorized(id, "http_ts") {
		return nil, ErrDenied
	}
	c.opens.Add(1)
	reader, writer := io.Pipe()
	go func() {
		defer writer.Close()
		ticker := time.NewTicker(time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if _, e := writer.Write(syntheticPacket()); e != nil {
					return
				}
			}
		}
	}()
	return reader, nil
}
func waitStatus(t *testing.T, m *Manager, state string) {
	t.Helper()
	until := time.Now().Add(4 * time.Second)
	for time.Now().Before(until) {
		for _, s := range m.Snapshot().Streams {
			if s.State == state && (state != "running" || s.Metrics.Bytes >= 1316) {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("missing state", state, m.Snapshot().Streams)
}
func TestContinuousSourceIsSharedByPreviewsAndStopsOnRevocation(t *testing.T) {
	catalog := &syntheticCatalog{}
	catalog.allowed.Store(true)
	manager, e := New(context.Background(), filepath.Join(t.TempDir(), "live"), catalog)
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	if catalog.opens.Load() != 0 {
		t.Fatal("source opened before explicit start")
	}
	if e = manager.Save(ChannelConfig{ChannelID: "synthetic-one", Protocol: "http_ts"}, Tools{}, 0); e != nil {
		t.Fatal(e)
	}
	if catalog.opens.Load() != 0 {
		t.Fatal("saving opened source")
	}
	if e = manager.Start("synthetic-one"); e != nil {
		t.Fatal(e)
	}
	waitStatus(t, manager, "running")
	if e = manager.Start("synthetic-one"); e != nil {
		t.Fatal(e)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !manager.MediaIfActive(w, r, "synthetic-one", "http_ts") {
			t.Error("missing ingest")
			w.WriteHeader(503)
		}
	}))
	defer server.Close()
	for i := 0; i < 2; i++ {
		response, e := http.Get(server.URL)
		if e != nil {
			t.Fatal(e)
		}
		bytes := make([]byte, 1316)
		_, e = io.ReadFull(response.Body, bytes)
		response.Body.Close()
		if e != nil || bytes[0] != 0x47 || response.Header.Get("X-Kore-Edge-Mode") != "continuous" {
			t.Fatal("preview unavailable", e)
		}
	}
	if catalog.opens.Load() != 1 {
		t.Fatal("preview opened a second source", catalog.opens.Load())
	}
	catalog.allowed.Store(false)
	waitStatus(t, manager, "unauthorized")
	response, e := http.Get(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatal("revoked preview allowed")
	}
}
func TestOutputPermissionsAndStaleConfigurationFailClosed(t *testing.T) {
	catalog := &syntheticCatalog{}
	catalog.allowed.Store(true)
	manager, e := New(context.Background(), filepath.Join(t.TempDir(), "live"), catalog)
	if e != nil {
		t.Fatal(e)
	}
	defer manager.Close()
	entry := ChannelConfig{ChannelID: "synthetic-one", Protocol: "http_ts", HLS: true}
	if manager.Save(entry, Tools{}, 0) != ErrDenied {
		t.Fatal("output permission expanded implicitly")
	}
	entry.HLS = false
	if manager.Save(entry, Tools{}, 0) != nil {
		t.Fatal("save")
	}
	if manager.Save(entry, Tools{}, 0) != ErrConflict {
		t.Fatal("stale revision")
	}
	if manager.StartReviewed(entry.ChannelID, 0) != ErrConflict {
		t.Fatal("stale start accepted")
	}
	if _, e = manager.PlanUDP(multicast.Target{ChannelID: entry.ChannelID}); e != ErrDenied {
		t.Fatal("UDP plan without grant")
	}
	if manager.Start("unconfigured") != ErrInvalid {
		t.Fatal("unconfigured start")
	}
	if catalog.opens.Load() != 0 {
		t.Fatal("validation started source")
	}
}
