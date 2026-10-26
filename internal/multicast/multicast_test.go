package multicast

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func target() Target { return Target{"test-pattern", "lab0", "10.250.0.2", "239.18.10.1", 12000, 1} }
func inventory() []Interface {
	return []Interface{{Name: "lab0", Index: 7, MTU: 1500, Up: true, Multicast: true, IPv4: []string{"10.250.0.2"}}}
}
func packet() []byte {
	b := bytes.Repeat([]byte{0xff}, 188)
	copy(b, []byte{0x47, 0x1f, 0xff, 0x10})
	return b
}

type fakeSocket struct {
	datagrams    [][]byte
	deadline     time.Time
	closed       bool
	fail         bool
	short        bool
	deadlineFail bool
}

func (s *fakeSocket) Write(b []byte) (int, error) {
	if s.fail {
		return 0, errors.New("synthetic failure")
	}
	if s.short {
		return len(b) - 1, nil
	}
	s.datagrams = append(s.datagrams, append([]byte(nil), b...))
	return len(b), nil
}
func (s *fakeSocket) SetWriteDeadline(d time.Time) error {
	s.deadline = d
	if s.deadlineFail {
		return errors.New("deadline")
	}
	return nil
}
func (s *fakeSocket) Close() error { s.closed = true; return nil }

func TestPacketizationAndClose(t *testing.T) {
	sock := &fakeSocket{}
	opened := 0
	s, e := New(context.Background(), target(), inventory(), func(tgt Target, index int) (Socket, error) {
		opened++
		if tgt != target() || index != 7 {
			t.Fatal("wrong interface/destination")
		}
		return sock, nil
	})
	if e != nil {
		t.Fatal(e)
	}
	for i := 0; i < 15; i++ {
		if e = s.WritePacket(packet()); e != nil {
			t.Fatal(e)
		}
	}
	stats := s.Snapshot()
	if opened != 1 || len(sock.datagrams) != 2 || stats.Bytes != 2*DatagramSize || stats.Datagrams != 2 || stats.BufferedPackets != 1 || stats.Errors != 0 {
		t.Fatalf("bad stats %+v", stats)
	}
	for _, b := range sock.datagrams {
		if len(b) != 1316 || !bytes.Equal(b, bytes.Repeat(packet(), 7)) {
			t.Fatal("bad datagram")
		}
	}
	if sock.deadline.IsZero() || time.Until(sock.deadline) > 100*time.Millisecond {
		t.Fatal("unbounded write")
	}
	if s.Close() != nil || s.Close() != nil || !sock.closed || s.Snapshot().BufferedPackets != 0 || len(sock.datagrams) != 2 {
		t.Fatal("close must discard partial batch")
	}
	if s.WritePacket(packet()) == nil {
		t.Fatal("write after close")
	}
}
func TestFailuresAndCancellation(t *testing.T) {
	for _, sock := range []*fakeSocket{{fail: true}, {short: true}, {deadlineFail: true}} {
		s, e := New(context.Background(), target(), inventory(), func(Target, int) (Socket, error) { return sock, nil })
		if e != nil {
			t.Fatal(e)
		}
		var err error
		for i := 0; i < 7; i++ {
			err = s.WritePacket(packet())
		}
		if err == nil || s.Snapshot().Errors != 1 || s.Snapshot().Datagrams != 0 {
			t.Fatal("write failure hidden")
		}
		s.Close()
		if !sock.closed {
			t.Fatal("socket leaked")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	if _, e := New(ctx, target(), inventory(), func(Target, int) (Socket, error) { called = true; return &fakeSocket{}, nil }); e == nil || called {
		t.Fatal("opened after cancellation")
	}
}
func TestValidationNoFallback(t *testing.T) {
	for _, change := range []func(*Target){func(t *Target) { t.Interface = "" }, func(t *Target) { t.SourceIP = "0.0.0.0" }, func(t *Target) { t.SourceIP = "8.8.8.8" }, func(t *Target) { t.Group = "224.0.0.1" }, func(t *Target) { t.Group = "10.0.0.1" }, func(t *Target) { t.Port = 0 }, func(t *Target) { t.TTL = 2 }, func(t *Target) { t.TTL = 0 }} {
		tgt := target()
		change(&tgt)
		if tgt.Validate() == nil {
			t.Fatal("invalid target accepted")
		}
	}
	for _, all := range [][]Interface{nil, {{Name: "lab0", Index: 7, MTU: 1500, Up: true, Multicast: true, IPv4: []string{"10.250.0.3"}}}, {{Name: "lab0", Index: 7, MTU: 1500, Up: false, Multicast: true, IPv4: []string{"10.250.0.2"}}}, {{Name: "lab0", Index: 7, MTU: 1500, Up: true, Multicast: true, PointToPoint: true, IPv4: []string{"10.250.0.2"}}}} {
		called := false
		if _, e := New(context.Background(), target(), all, func(Target, int) (Socket, error) { called = true; return &fakeSocket{}, nil }); e == nil || called {
			t.Fatal("fallback used")
		}
	}
	c := Config{Mode: "send", Outputs: []Target{target(), target()}}
	if c.Validate() == nil {
		t.Fatal("duplicate targets")
	}
	if (Config{}).Validate() != nil {
		t.Fatal("disabled config")
	}
}

type fakeOptions struct {
	calls  []string
	failAt int
	t      *testing.T
}

func (o *fakeOptions) add(name string) error {
	o.calls = append(o.calls, name)
	if len(o.calls) == o.failAt {
		return errors.New("option failed")
	}
	return nil
}
func (o *fakeOptions) Device(s string) error {
	if s != "lab0" {
		o.t.Fatal("device")
	}
	return o.add("device")
}
func (o *fakeOptions) Interface(index int, ip string) error {
	if index != 7 || ip != "10.250.0.2" {
		o.t.Fatal("interface pin")
	}
	return o.add("interface")
}
func (o *fakeOptions) TTL(n int) error {
	if n != 1 {
		o.t.Fatal("TTL")
	}
	return o.add("ttl")
}
func (o *fakeOptions) Loopback(on bool) error {
	if on {
		o.t.Fatal("loopback enabled")
	}
	return o.add("loopback")
}
func (o *fakeOptions) Buffer(n int) error {
	if n != 65536 {
		o.t.Fatal("buffer")
	}
	return o.add("buffer")
}
func TestOptionsFailClosed(t *testing.T) {
	for fail := 0; fail <= 5; fail++ {
		o := &fakeOptions{failAt: fail, t: t}
		e := applyOptions(target(), 7, o)
		if fail == 0 {
			if e != nil || !reflect.DeepEqual(o.calls, []string{"device", "interface", "ttl", "loopback", "buffer"}) {
				t.Fatal("option sequence")
			}
		} else if e == nil || len(o.calls) != fail {
			t.Fatal("continued after failed pin")
		}
	}
}
func TestDryRun(t *testing.T) {
	p, e := DryRun(target(), inventory())
	if e != nil || p.Applied || p.Mode != "dry_run" || p.InterfaceIndex != 7 || p.DatagramBytes != 1316 || len(p.Rollback) == 0 || len(p.Validation) == 0 {
		t.Fatal("invalid plan")
	}
}
