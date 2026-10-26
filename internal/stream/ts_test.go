package stream

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
)

func nullPacket() []byte {
	p := bytes.Repeat([]byte{0xff}, PacketSize)
	copy(p, []byte{0x47, 0x1f, 0xff, 0x10})
	return p
}

func TestIngestCounters(t *testing.T) {
	packet := nullPacket()
	packet[1] |= 0x80
	var b, p, s, tei uint64
	e := ingest(bytes.NewReader(append(nullPacket(), packet...)), func(nb, np, ns, nt uint64) { b += nb; p += np; s += ns; tei += nt })
	if e != nil || b != 376 || p != 2 || s != 0 || tei != 1 {
		t.Fatalf("bytes=%d packets=%d sync=%d TEI=%d error=%v", b, p, s, tei, e)
	}
}
func TestRejectMalformedTS(t *testing.T) {
	for _, input := range [][]byte{make([]byte, PacketSize), nullPacket()[:30], func() []byte { p := nullPacket(); p[3] = 0; return p }(), func() []byte { p := nullPacket(); p[3] = 0x30; p[4] = 183; return p }()} {
		var syncErrors uint64
		if ingest(bytes.NewReader(input), func(b, p, s, tei uint64) { syncErrors += s }) == nil || syncErrors != 1 {
			t.Fatal("accepted corrupt TS")
		}
	}
}

type cancelWriter struct {
	cancel context.CancelFunc
	data   []byte
}

func (w *cancelWriter) Write(p []byte) (int, error) {
	w.data = append(w.data, p...)
	w.cancel()
	return len(p), nil
}
func TestGenerator(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	w := &cancelWriter{cancel: cancel}
	if !errors.Is(Generate(ctx, w), context.Canceled) {
		t.Fatal("cancellation")
	}
	var packets uint64
	if e := ingest(bytes.NewReader(w.data), func(b, p, s, tei uint64) { packets += p }); e != nil || packets != 7 {
		t.Fatal("invalid synthetic output")
	}
	if !bytes.Equal(w.data[:PacketSize], nullPacket()) {
		t.Fatal("unexpected PID")
	}
}

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return 0, nil }
func TestGeneratorShortWrite(t *testing.T) {
	if !errors.Is(Generate(context.Background(), shortWriter{}), io.ErrShortWrite) {
		t.Fatal("short write")
	}
}
