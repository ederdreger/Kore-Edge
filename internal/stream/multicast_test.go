package stream

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"kore-edge/internal/multicast"
)

type mockDatagramSocket struct {
	closed atomic.Bool
	fail   bool
}

func (s *mockDatagramSocket) Write(p []byte) (int, error) {
	if s.fail {
		return 0, errors.New("synthetic failure")
	}
	return len(p), nil
}
func (s *mockDatagramSocket) SetWriteDeadline(time.Time) error { return nil }
func (s *mockDatagramSocket) Close() error                     { s.closed.Store(true); return nil }
func mockSender(ctx context.Context, sock *mockDatagramSocket) (Output, error) {
	return multicast.New(ctx, multicast.Target{ChannelID: "test", Interface: "lab0", SourceIP: "10.250.0.2", Group: "239.18.10.1", Port: 12000, TTL: 1}, []multicast.Interface{{Name: "lab0", Index: 7, MTU: 1500, Up: true, Multicast: true, IPv4: []string{"10.250.0.2"}}}, func(multicast.Target, int) (multicast.Socket, error) { return sock, nil })
}
func TestMulticastSharesSingleIngest(t *testing.T) {
	var sources atomic.Int32
	sock := &mockDatagramSocket{}
	e, err := NewWithOutput(context.Background(), func(ctx context.Context) (*Process, error) { sources.Add(1); return fakeSource(ctx) }, allowAll, testPolicy(), func(ctx context.Context, id string) (Output, error) { return mockSender(ctx, sock) })
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	_ = e.Start("test")
	_ = e.Start("test")
	s := waitStatus(t, e, func(s []Status) bool { return len(s) == 1 && s[0].UDP != nil && s[0].UDP.Datagrams > 0 })[0]
	if sources.Load() != 1 || s.UDP.Bytes != s.UDP.Datagrams*1316 {
		t.Fatal("duplicate ingest or bad output counts")
	}
	e.Close()
	if !sock.closed.Load() {
		t.Fatal("socket not closed")
	}
}
func TestOutputFailureDoesNotStopOtherChannel(t *testing.T) {
	policy := testPolicy()
	policy.MaxRestarts = 1
	e, err := NewWithOutput(context.Background(), fakeSource, allowAll, policy, func(ctx context.Context, id string) (Output, error) {
		if id != "bad" {
			return nil, nil
		}
		return mockSender(ctx, &mockDatagramSocket{fail: true})
	})
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	_ = e.Start("bad")
	_ = e.Start("good")
	s := waitStatus(t, e, func(s []Status) bool { return len(s) == 2 && s[0].State == "failed" && s[1].State == "running" })
	if s[0].Attempts != 2 || s[0].UDP == nil || s[0].UDP.Errors != 1 || s[1].Packets == 0 {
		t.Fatal("failure isolation")
	}
}
