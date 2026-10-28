package live

import (
	"context"
	"sync"
)

// isolatedOutput never waits for a segmenter's pipe in the source read loop.
// Saturation fails this output as a whole, rather than dropping TS packets into
// a corrupt segment or blocking reception. Recovery resamples the shared input.
type isolatedOutput struct {
	ctx    context.Context
	cancel context.CancelFunc
	output Output
	queue  chan [188]byte
	done   chan struct{}
	once   sync.Once
}

func isolateOutput(ctx context.Context, output Output) *isolatedOutput {
	ctx, cancel := context.WithCancel(ctx)
	r := &isolatedOutput{ctx: ctx, cancel: cancel, output: output, queue: make(chan [188]byte, 1024), done: make(chan struct{})}
	go func() {
		defer close(r.done)
		for {
			select {
			case <-ctx.Done():
				return
			case packet := <-r.queue:
				if output.WritePacket(packet[:]) != nil {
					cancel()
					return
				}
			}
		}
	}()
	return r
}
func (r *isolatedOutput) WritePacket(packet []byte) error {
	if len(packet) != 188 || r.ctx.Err() != nil {
		return ErrUnavailable
	}
	var copyPacket [188]byte
	copy(copyPacket[:], packet)
	select {
	case <-r.ctx.Done():
		return ErrUnavailable
	case r.queue <- copyPacket:
		return nil
	default:
		r.cancel()
		return ErrUnavailable
	}
}
func (r *isolatedOutput) Close() error {
	r.once.Do(func() { r.cancel(); r.output.Close(); <-r.done })
	return nil
}
