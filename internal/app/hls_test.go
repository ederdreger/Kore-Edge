package app

import (
	"errors"
	"testing"

	"kore-edge/internal/multicast"
)

type testUDP struct {
	fail   bool
	closed bool
}

func (o *testUDP) WritePacket([]byte) error {
	if o.fail {
		return errors.New("synthetic failure")
	}
	return nil
}
func (o *testUDP) Close() error              { o.closed = true; return nil }
func (o *testUDP) Snapshot() multicast.Stats { return multicast.Stats{State: "test_fixture"} }
func TestFanoutLifecycle(t *testing.T) {
	good := &testUDP{}
	f := &fanout{udp: good}
	if f.WritePacket(make([]byte, 188)) != nil {
		t.Fatal("healthy output")
	}
	_ = f.Close()
	if !good.closed {
		t.Fatal("socket not closed")
	}
	bad := &testUDP{fail: true}
	f = &fanout{udp: bad}
	if f.WritePacket(make([]byte, 188)) == nil || !bad.closed || f.udp != nil {
		t.Fatal("failed output not isolated")
	}
}
