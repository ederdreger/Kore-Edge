package stream

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestFailureBudgetDoesNotInterruptHealthyStream(t *testing.T) {
	var factories atomic.Int32
	engine, e := NewWithOutput(context.Background(), func(ctx context.Context) (*Process, error) { factories.Add(1); return fakeSource(ctx) }, allowAll, testPolicy(), func(ctx context.Context, id string) (Output, error) {
		if id == "failing" {
			return nil, errors.New("synthetic output failure")
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
	before := waitStatus(t, engine, func(s []Status) bool { return len(s) == 1 && s[0].Packets >= 7 })[0]
	if engine.Start("failing") != nil {
		t.Fatal("failing start")
	}
	waitStatus(t, engine, func(s []Status) bool { return len(s) == 2 && s[0].State == "failed" })
	after := waitStatus(t, engine, func(s []Status) bool { return len(s) == 2 && s[1].Packets > before.Packets })
	if after[1].State != "running" || after[1].Attempts != 1 || factories.Load() != 1 || after[0].Attempts != testPolicy().MaxRestarts+1 {
		t.Fatal("failure affected healthy stream", after)
	}
}
func TestRetryScheduleBoundedAndCancellation(t *testing.T) {
	p := DefaultPolicy()
	for attempt, want := range []time.Duration{250 * time.Millisecond, 500 * time.Millisecond, time.Second, 2 * time.Second} {
		if p.retryDelay(attempt) != want {
			t.Fatal("unexpected exponential delay")
		}
	}
	if p.retryDelay(1000000) != 30*time.Second {
		t.Fatal("backoff not capped")
	}
	p.Backoff = 30 * time.Second
	p.MaxRestarts = 1
	engine, e := New(context.Background(), func(context.Context) (*Process, error) { return nil, errors.New("synthetic failure") }, allowAll, p)
	if e != nil {
		t.Fatal(e)
	}
	if engine.Start("test") != nil {
		t.Fatal("start")
	}
	waitStatus(t, engine, func(s []Status) bool { return len(s) == 1 && s[0].State == "backoff" })
	done := make(chan struct{})
	go func() { engine.Close(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("backoff ignored cancellation")
	}
}
