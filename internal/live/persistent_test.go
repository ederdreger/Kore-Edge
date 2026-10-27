package live

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestPersistentTSKeepsOneHTTPConnectionAcrossIngestRecovery(t *testing.T) {
	c := &syntheticCatalog{}
	c.allowed.Store(true)
	m, e := New(context.Background(), filepath.Join(t.TempDir(), "live"), c)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	drop := make(chan struct{})
	var opens atomic.Int32
	m.retryDelay = func(int) time.Duration { return 1500 * time.Millisecond }
	m.open = func(ctx context.Context, _ ChannelConfig, _ Tools) (io.ReadCloser, error) {
		n := opens.Add(1)
		r, w := io.Pipe()
		go func() {
			defer w.Close()
			failure := drop
			if n != 1 {
				failure = nil
			}
			ticker := time.NewTicker(time.Millisecond)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-failure:
					if n == 1 {
						return
					}
				case <-ticker.C:
					data := make([]byte, 188*7)
					for i := 0; i < len(data); i += 188 {
						copy(data[i:], syntheticPacket())
						data[i+1] = 1
						data[i+2] = byte(n - 1)
					}
					if _, e := w.Write(data); e != nil {
						return
					}
				}
			}
		}()
		return r, nil
	}
	if m.Save(ChannelConfig{ChannelID: "synthetic", Protocol: "http_ts"}, Tools{}, 0) != nil || m.Start("synthetic") != nil {
		t.Fatal("source setup")
	}
	waitStatus(t, m, "running")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.ServePersistentTS(w, r, "synthetic") }))
	defer server.Close()
	client := server.Client()
	client.Timeout = 8 * time.Second
	response, e := client.Get(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatal(response.StatusCode)
	}
	packet := make([]byte, 188)
	if _, e = io.ReadFull(response.Body, packet); e != nil {
		t.Fatal(e)
	}
	m.mu.Lock()
	job := m.jobs["synthetic"]
	m.mu.Unlock()
	job.mu.Lock()
	oldHub := job.hub
	job.mu.Unlock()
	close(drop)
	waitStatus(t, m, "backoff")
	frozen := oldHub.Snapshot().Bytes
	padding := false
	recovered := false
	for !recovered {
		if _, e = io.ReadFull(response.Body, packet); e != nil {
			t.Fatal("HTTP stream ended on ingest retry", e)
		}
		pid := int(packet[1]&0x1f)<<8 | int(packet[2])
		if pid == 0x1fff {
			padding = true
			if oldHub.Snapshot().Bytes != frozen {
				t.Fatal("idle padding polluted source metrics")
			}
		}
		if pid == 257 {
			recovered = true
		}
	}
	if !padding || opens.Load() != 2 {
		t.Fatal("missing transport keepalive or duplicate source", padding, opens.Load())
	}
	if e = m.Stop("synthetic"); e != nil {
		t.Fatal(e)
	}
	start := time.Now()
	_, _ = io.Copy(io.Discard, response.Body)
	if time.Since(start) > 2*time.Second {
		t.Fatal("manual stop did not close persistent output")
	}
}

func TestPersistentTSClosesOnAgentShutdownWithoutClearingResume(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c := &syntheticCatalog{}
	c.allowed.Store(true)
	m, e := New(ctx, filepath.Join(t.TempDir(), "live"), c)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	if m.Save(ChannelConfig{ChannelID: "synthetic", Protocol: "http_ts", Resume: true}, Tools{}, 0) != nil || m.Start("synthetic") != nil {
		t.Fatal("setup")
	}
	waitStatus(t, m, "running")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.ServePersistentTS(w, r, "synthetic") }))
	defer server.Close()
	client := server.Client()
	client.Timeout = 3 * time.Second
	response, e := client.Get(server.URL)
	if e != nil {
		t.Fatal(e)
	}
	defer response.Body.Close()
	cancel()
	start := time.Now()
	_, _ = io.Copy(io.Discard, response.Body)
	if time.Since(start) > time.Second {
		t.Fatal("shutdown left an idle HTTP response alive")
	}
	if !m.Snapshot().Configuration.Channels[0].Resume {
		t.Fatal("shutdown cleared desired reception")
	}
}
