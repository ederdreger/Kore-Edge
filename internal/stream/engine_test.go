package stream

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func testPolicy() Policy {
	return Policy{2, 4, 2, 5 * time.Millisecond, 150 * time.Millisecond, 10 * time.Millisecond}
}
func allowAll(string, time.Time) bool { return true }
func waitStatus(t *testing.T, e *Engine, predicate func([]Status) bool) []Status {
	t.Helper()
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Millisecond)
	defer ticker.Stop()
	for {
		status := e.Snapshot()
		if predicate(status) {
			return status
		}
		select {
		case <-deadline.C:
			t.Fatalf("timeout: %+v", status)
		case <-ticker.C:
		}
	}
}
func fakeSource(ctx context.Context) (*Process, error) {
	child, cancel := context.WithCancel(ctx)
	r, w := io.Pipe()
	done := make(chan error, 1)
	go func() { e := Generate(child, w); w.Close(); done <- e }()
	return &Process{Output: r, Kill: func() error { cancel(); return w.Close() }, Wait: func() error { return <-done }}, nil
}

func TestFlowAndIdempotentLifecycle(t *testing.T) {
	var calls atomic.Int32
	e, err := New(context.Background(), func(ctx context.Context) (*Process, error) { calls.Add(1); return fakeSource(ctx) }, allowAll, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Start("test") != nil || e.Start("test") != nil {
		t.Fatal("start")
	}
	s := waitStatus(t, e, func(s []Status) bool {
		return len(s) == 1 && s[0].Packets >= 7 && s[0].Bitrate.BitsPerSecond != nil && *s[0].Bitrate.BitsPerSecond > 0
	})[0]
	if s.Packets*PacketSize != s.Bytes || s.SyncErrors != 0 || s.TransportErrors != 0 || s.Source != "synthetic:null-ts" || s.LastPacketAt == nil || s.Bitrate.State != "measured_synthetic" || s.Bitrate.WindowSeconds <= 0 || calls.Load() != 1 {
		t.Fatalf("bad measurements: %+v", s)
	}
	if !e.Stop("test") || !e.Stop("test") {
		t.Fatal("idempotent stop")
	}
	waitStatus(t, e, func(s []Status) bool { return s[0].State == "stopped" && s[0].Bitrate.BitsPerSecond == nil })
	if e.Stop("missing") {
		t.Fatal("unknown stop")
	}
}

func TestBoundedRestarts(t *testing.T) {
	var calls atomic.Int32
	e, err := New(context.Background(), func(context.Context) (*Process, error) { calls.Add(1); return nil, errors.New("source failed") }, allowAll, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Start("test") != nil {
		t.Fatal("start")
	}
	s := waitStatus(t, e, func(s []Status) bool { return len(s) == 1 && s[0].State == "failed" })[0]
	if calls.Load() != 3 || s.Attempts != 3 || s.Restarts != 2 {
		t.Fatalf("unbounded restarts: %+v", s)
	}
}

func TestWatchdog(t *testing.T) {
	var killed atomic.Int32
	factory := func(ctx context.Context) (*Process, error) {
		r, w := io.Pipe()
		done := make(chan struct{})
		return &Process{Output: r, Kill: func() error {
			killed.Add(1)
			select {
			case <-done:
			default:
				close(done)
			}
			return w.Close()
		}, Wait: func() error { <-done; return nil }}, nil
	}
	p := testPolicy()
	p.MaxRestarts = 0
	e, err := New(context.Background(), factory, allowAll, p)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	_ = e.Start("test")
	s := waitStatus(t, e, func(s []Status) bool { return len(s) == 1 && s[0].State == "failed" })[0]
	if killed.Load() == 0 || s.Bytes != 0 || s.Bitrate.BitsPerSecond != nil {
		t.Fatal("watchdog did not reap source")
	}
}

func TestAuthorizationAndLimits(t *testing.T) {
	var allowed atomic.Bool
	allowed.Store(true)
	e, err := New(context.Background(), fakeSource, func(id string, _ time.Time) bool { return id != "denied" && allowed.Load() }, testPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if !errors.Is(e.Start("denied"), ErrDenied) {
		t.Fatal("authorization bypass")
	}
	_ = e.Start("one")
	_ = e.Start("two")
	if !errors.Is(e.Start("three"), ErrLimit) {
		t.Fatal("limit bypass")
	}
	waitStatus(t, e, func(s []Status) bool { return len(s) == 2 && s[0].Packets > 0 && s[1].Packets > 0 })
	allowed.Store(false)
	waitStatus(t, e, func(s []Status) bool {
		return len(s) == 2 && s[0].State == "unauthorized" && s[1].State == "unauthorized"
	})
	e.Close()
	if !errors.Is(e.Start("one"), ErrClosed) {
		t.Fatal("closed engine restarted")
	}
}

// This helper runs only when invoked as the supervised child. No network, shell
// or external media tool is involved; it emits the same generator as the agent.
func TestSourceProcess(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != "ts-child" {
		return
	}
	_ = Generate(context.Background(), os.Stdout)
	os.Exit(0)
}
func TestRealChildProcess(t *testing.T) {
	e, err := New(context.Background(), commandFactory(os.Args[0], "-test.run=TestSourceProcess", "--", "ts-child"), allowAll, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Start("test") != nil {
		t.Fatal("start")
	}
	waitStatus(t, e, func(s []Status) bool { return len(s) == 1 && s[0].Packets >= 7 && s[0].Bitrate.BitsPerSecond != nil })
	e.Close()
	s := e.Snapshot()
	if s[0].State != "stopped" {
		t.Fatal("child not reaped")
	}
}

func TestBuiltAgentSource(t *testing.T) {
	path := os.Getenv("KORE_EDGE_AGENT_TEST_BINARY")
	if path == "" {
		t.Skip("set KORE_EDGE_AGENT_TEST_BINARY for built-agent integration")
	}
	e, err := New(context.Background(), commandFactory(path, "-synthetic-ts"), allowAll, DefaultPolicy())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if e.Start("test") != nil {
		t.Fatal("start")
	}
	s := waitStatus(t, e, func(s []Status) bool { return len(s) == 1 && s[0].Packets >= 7 && s[0].Bitrate.BitsPerSecond != nil })[0]
	if s.Bytes != s.Packets*PacketSize || s.SyncErrors != 0 || s.TransportErrors != 0 || *s.Bitrate.BitsPerSecond <= 0 {
		t.Fatalf("invalid agent output: %+v", s)
	}
	e.Close()
	if e.Snapshot()[0].State != "stopped" {
		t.Fatal("agent child not reaped")
	}
}

func TestStopDuringBackoff(t *testing.T) {
	var attempts atomic.Int32
	p := testPolicy()
	p.Backoff = time.Second
	e, err := New(context.Background(), func(context.Context) (*Process, error) { attempts.Add(1); return nil, errors.New("failure") }, allowAll, p)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	_ = e.Start("test")
	waitStatus(t, e, func(s []Status) bool { return len(s) == 1 && s[0].State == "backoff" })
	e.Stop("test")
	waitStatus(t, e, func(s []Status) bool { return s[0].State == "stopped" })
	if attempts.Load() != 1 {
		t.Fatal("restart after stop")
	}
}

func TestExitAndCorruptSourcesHaveFiniteBudget(t *testing.T) {
	for _, data := range [][]byte{nullPacket(), make([]byte, PacketSize)} {
		var calls atomic.Int32
		factory := func(context.Context) (*Process, error) {
			calls.Add(1)
			return &Process{Output: io.NopCloser(bytes.NewReader(data)), Wait: func() error { return nil }, Kill: func() error { return nil }}, nil
		}
		e, err := New(context.Background(), factory, allowAll, testPolicy())
		if err != nil {
			t.Fatal(err)
		}
		_ = e.Start("test")
		s := waitStatus(t, e, func(s []Status) bool { return len(s) == 1 && s[0].State == "failed" })[0]
		e.Close()
		if calls.Load() != 3 || s.Attempts != 3 {
			t.Fatal("exit restart budget")
		}
		if data[0] == 0 && s.SyncErrors != 3 {
			t.Fatal("corruption not counted")
		}
	}
}

func TestHistoryBound(t *testing.T) {
	p := testPolicy()
	p.MaxActive = 1
	p.MaxHistory = 1
	p.MaxRestarts = 0
	e, err := New(context.Background(), func(context.Context) (*Process, error) { return nil, errors.New("failure") }, allowAll, p)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	_ = e.Start("one")
	waitStatus(t, e, func(s []Status) bool { return len(s) == 1 && s[0].State == "failed" })
	if !errors.Is(e.Start("two"), ErrLimit) {
		t.Fatal("unbounded history")
	}
}
