package stream

import (
	"context"
	"io"
	"time"
)

// Generate emits only synthetic MPEG-TS null packets (PID 8191), not media.
// Seven packets per 10 ms targets 1,052,800 bit/s; actual ingress is measured.
func Generate(ctx context.Context, w io.Writer) error {
	var batch [7 * PacketSize]byte
	for i := 0; i < 7; i++ {
		p := batch[i*PacketSize : (i+1)*PacketSize]
		for j := range p {
			p[j] = 0xff
		}
		p[0] = 0x47
		p[1] = 0x1f
		p[2] = 0xff
		p[3] = 0x10
	}
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			n, e := w.Write(batch[:])
			if e != nil {
				return e
			}
			if n != len(batch) {
				return io.ErrShortWrite
			}
		}
	}
}
