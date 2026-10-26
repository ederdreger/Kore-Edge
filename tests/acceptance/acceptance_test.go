// Package acceptance_test exercises bounded laboratory scenarios only.
package acceptance_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kore-edge/internal/app"
	"kore-edge/internal/diagnostics"
	"kore-edge/internal/monitoring"
	"kore-edge/internal/stream"
	"kore-edge/internal/tsmetrics"
)

func TestBoundedHTTPReadLoad(t *testing.T) {
	listener, e := net.Listen("tcp", "127.0.0.1:0")
	if e != nil {
		t.Fatal(e)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- app.Serve(ctx, listener) }()
	defer func() {
		cancel()
		select {
		case e := <-done:
			if e != nil {
				t.Error(e)
			}
		case <-time.After(7 * time.Second):
			t.Error("core did not stop")
		}
	}()
	transport := &http.Transport{MaxConnsPerHost: 8, MaxIdleConnsPerHost: 8}
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: 3 * time.Second}
	var failures atomic.Int64
	var longest atomic.Int64
	var completed atomic.Int64
	start := time.Now()
	var wg sync.WaitGroup
	paths := []string{"/healthz", "/readyz", "/api/v1/session", "/api/v1/integration"}
	for worker := 0; worker < 8; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 64; n++ {
				at := time.Now()
				response, e := client.Get("http://" + listener.Addr().String() + paths[n%len(paths)])
				if e != nil {
					failures.Add(1)
					continue
				}
				_, copyErr := io.Copy(io.Discard, io.LimitReader(response.Body, 8193))
				response.Body.Close()
				if response.StatusCode != 200 || copyErr != nil {
					failures.Add(1)
				}
				completed.Add(1)
				elapsed := time.Since(at).Nanoseconds()
				for old := longest.Load(); elapsed > old; old = longest.Load() {
					if longest.CompareAndSwap(old, elapsed) {
						break
					}
				}
			}
		}()
	}
	wg.Wait()
	if failures.Load() != 0 || completed.Load() != 512 {
		t.Fatalf("read requests: completed=%d failures=%d", completed.Load(), failures.Load())
	}
	t.Logf("synthetic loopback load: requests=512 workers=8 elapsed=%s longest=%s (observation, not SLA)", time.Since(start), time.Duration(longest.Load()))
}

func TestSyntheticTransportLossAndAlert(t *testing.T) {
	now := time.Now().UTC()
	analyzer := tsmetrics.New(now)
	delivered := 0
	// Two separated single-packet omissions, not a NIC or UDP impairment.
	for sequence := 0; sequence < 40; sequence++ {
		if sequence == 7 || sequence == 26 {
			continue
		}
		packet := bytes.Repeat([]byte{0xff}, 188)
		packet[0] = 0x47
		packet[1] = 1
		packet[2] = 0
		packet[3] = 0x10 | byte(sequence&15)
		packet[4] = byte(sequence)
		analyzer.Observe(packet, now)
		delivered++
	}
	snapshot := analyzer.Snapshot()
	if delivered != 38 || snapshot.CCErrors != 2 || snapshot.CCState != "measured" || snapshot.Malformed != 0 {
		t.Fatalf("synthetic continuity: delivered=%d cc_errors=%d", delivered, snapshot.CCErrors)
	}
	service := monitoring.New(func() []stream.Status { return nil }, nil)
	service.Sample(now, []stream.Status{{ChannelID: "synthetic-loss", State: "running", Source: "synthetic:acceptance-loss", StartedAt: now.Add(-time.Second), LastPacketAt: &now, Attempts: 1, Transport: snapshot}}, diagnostics.Metric{State: "unavailable", Timestamp: now})
	found := false
	for _, alert := range service.Snapshot(now).Alerts {
		if alert.Code == "continuity_errors" && alert.State == "firing" {
			found = true
		}
	}
	if !found {
		t.Fatal("loss did not generate continuity alert")
	}
	t.Log("injected omissions=2 delivered=38 continuity gaps=2; CC gaps are not a network packet-loss percentage")
}

func source(ctx context.Context) (*stream.Process, error) {
	child, cancel := context.WithCancel(ctx)
	reader, writer := io.Pipe()
	done := make(chan error, 1)
	go func() { e := stream.Generate(child, writer); writer.Close(); done <- e }()
	return &stream.Process{Output: reader, Kill: func() error { cancel(); return writer.Close() }, Wait: func() error { return <-done }}, nil
}
func await(t *testing.T, engine *stream.Engine, predicate func([]stream.Status) bool) []stream.Status {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		snapshot := engine.Snapshot()
		if predicate(snapshot) {
			return snapshot
		}
		select {
		case <-deadline.C:
			t.Fatal("stream acceptance deadline")
		case <-ticker.C:
		}
	}
}
func TestRecoveryAndCapacityPreserveHealthyChannel(t *testing.T) {
	var faults atomic.Int32
	policy := stream.DefaultPolicy()
	policy.MaxActive = 2
	policy.Backoff = 20 * time.Millisecond
	engine, e := stream.NewWithOutput(context.Background(), source, func(string, time.Time) bool { return true }, policy, func(ctx context.Context, id string) (stream.Output, error) {
		if id == "recover" && faults.Add(1) == 1 {
			return nil, errors.New("synthetic transient failure")
		}
		return nil, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	defer engine.Close()
	if engine.Start("healthy") != nil {
		t.Fatal("healthy start")
	}
	before := await(t, engine, func(s []stream.Status) bool { return len(s) == 1 && s[0].Packets >= 7 })[0]
	if engine.Start("recover") != nil {
		t.Fatal("recover start")
	}
	if !errors.Is(engine.Start("over-capacity"), stream.ErrLimit) {
		t.Fatal("active-stream cap ignored")
	}
	after := await(t, engine, func(s []stream.Status) bool {
		return len(s) == 2 && s[1].State == "running" && s[1].Packets >= 7 && s[0].Packets > before.Packets
	})
	if after[0].Attempts != 1 || after[0].State != "running" || after[1].Attempts != 2 || after[1].Restarts != 1 {
		t.Fatal("transient recovery affected healthy channel")
	}
	if engine.Start("recover") != nil {
		t.Fatal("idempotent start")
	}
	if !engine.Stop("recover") {
		t.Fatal("stop")
	}
	await(t, engine, func(s []stream.Status) bool { return s[1].State == "stopped" })
	t.Log("active cap=2; recovered attempts=2 restarts=1; healthy attempts=1")
}
