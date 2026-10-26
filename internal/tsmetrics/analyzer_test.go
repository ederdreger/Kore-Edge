package tsmetrics

import (
	"bytes"
	"math"
	"testing"
	"time"
)

func packet(pid uint16, cc byte) []byte {
	b := bytes.Repeat([]byte{0xff}, 188)
	b[0] = 0x47
	b[1] = byte(pid>>8) & 31
	b[2] = byte(pid)
	b[3] = 0x10 | (cc & 15)
	return b
}
func pcrPacket(pid uint16, cc byte, ticks uint64, discontinuity bool) []byte {
	b := packet(pid, cc)
	b[3] = 0x30 | (cc & 15)
	b[4] = 7
	b[5] = 0x10
	if discontinuity {
		b[5] |= 0x80
	}
	base, ext := ticks/300, ticks%300
	b[6] = byte(base >> 25)
	b[7] = byte(base >> 17)
	b[8] = byte(base >> 9)
	b[9] = byte(base >> 1)
	b[10] = byte(base&1)<<7 | 0x7e | byte(ext>>8)
	b[11] = byte(ext)
	return b
}

func TestContinuityAndDuplicates(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	a := New(now)
	for _, b := range [][]byte{packet(256, 14), packet(257, 7), packet(256, 15), packet(256, 0), packet(256, 0), packet(256, 1), packet(256, 4)} {
		a.Observe(b, now)
	}
	s := a.Snapshot()
	if s.CCErrors != 1 || s.DuplicatePackets != 1 || s.ContinuityPackets != 7 || s.CCState != "measured" {
		t.Fatalf("bad CC %+v", s)
	}
	b := packet(256, 9)
	b[3] = 0x20 | 9
	b[4] = 183
	b[5] = 0x80
	a.Observe(b, now)
	a.Observe(packet(256, 10), now)
	s = a.Snapshot()
	if s.CCErrors != 1 || s.Discontinuities != 1 {
		t.Fatal("discontinuity not respected")
	}
	// Adaptation-only packets keep CC unchanged.
	b = packet(256, 10)
	b[3] = 0x20 | 10
	b[4] = 183
	b[5] = 0
	a.Observe(b, now)
	a.Observe(packet(256, 11), now)
	if a.Snapshot().CCErrors != 1 {
		t.Fatal("adaptation-only CC")
	}
}
func TestNullTEIAndMalformed(t *testing.T) {
	now := time.Now().UTC()
	a := New(now)
	a.Observe(packet(8191, 0), now)
	a.Observe(packet(8191, 12), now)
	if a.Snapshot().CCState != "unavailable" || a.Snapshot().PCRIntervalSeconds != nil {
		t.Fatal("null stream quality invented")
	}
	b := packet(256, 0)
	b[1] |= 0x80
	a.Observe(b, now)
	a.Observe(packet(256, 8), now)
	a.Observe([]byte{0}, now)
	s := a.Snapshot()
	if s.TEI != 1 || s.CCErrors != 0 || s.Malformed != 1 {
		t.Fatal("TEI/malformed handling")
	}
}

func TestOnlyOneConsecutiveDuplicateIsAccepted(t *testing.T) {
	now := time.Now().UTC()
	a := New(now)
	for i := 0; i < 4; i++ {
		a.Observe(packet(256, 0), now)
	}
	s := a.Snapshot()
	if s.DuplicatePackets != 1 || s.CCErrors != 2 {
		t.Fatalf("duplicate handling: %+v", s)
	}
}
func TestPCRIntervalWrapAndDiscontinuity(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	a := New(now)
	a.Observe(pcrPacket(256, 0, PCRModulus-2700000, false), now)
	if a.Snapshot().PCRIntervalSeconds != nil {
		t.Fatal("pair needed")
	}
	a.Observe(pcrPacket(257, 0, 500000, false), now.Add(time.Millisecond))
	if a.Snapshot().PCRIntervalSeconds != nil {
		t.Fatal("PCRs from different PIDs combined")
	}
	a.Observe(pcrPacket(256, 1, 0, false), now.Add(100*time.Millisecond))
	s := a.Snapshot()
	if s.PCRWraps != 1 || s.PCRCount != 3 || s.PCRIntervalSeconds == nil || math.Abs(*s.PCRIntervalSeconds-0.1) > 1e-9 || s.PCRPairStart == nil || !s.PCRPairStart.Equal(now) {
		t.Fatalf("PCR %+v", s)
	}
	a.Observe(pcrPacket(256, 2, 2700000, false), now.Add(200*time.Millisecond))
	a.Observe(pcrPacket(256, 3, 270000, false), now.Add(300*time.Millisecond))
	if a.Snapshot().PCRBackwards != 1 || a.Snapshot().PCRIntervalSeconds != nil {
		t.Fatal("backward PCR")
	}
	a.Observe(pcrPacket(256, 4, 500, false), now.Add(time.Second)) // another backward clock
	a.Observe(pcrPacket(256, 5, 200, false), now.Add(2*time.Second))
	a.Observe(pcrPacket(256, 6, 0, true), now.Add(3*time.Second))
	s = a.Snapshot()
	if s.PCRIntervalSeconds != nil || s.Discontinuities != 1 {
		t.Fatal("PCR discontinuity")
	}
}
func TestPCRInvalidAndSnapshotIsolation(t *testing.T) {
	now := time.Now().UTC()
	a := New(now)
	b := pcrPacket(256, 0, 1, false)
	b[10] = 0
	b[11] = 255
	a.Observe(b, now)
	if a.Snapshot().PCRInvalid != 1 {
		t.Fatal("reserved PCR bits")
	}
	a.Observe(pcrPacket(256, 1, 0, false), now)
	a.Observe(pcrPacket(256, 2, 2700000, false), now.Add(time.Second))
	s := a.Snapshot()
	*s.PCRPID = 99
	*s.PCRIntervalSeconds = 99
	if *a.Snapshot().PCRPID == 99 || *a.Snapshot().PCRIntervalSeconds == 99 {
		t.Fatal("mutable snapshot")
	}
}
