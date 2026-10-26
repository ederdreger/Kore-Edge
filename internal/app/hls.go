package app

import (
	"context"
	"errors"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/hls"
	"kore-edge/internal/multicast"
	"kore-edge/internal/stream"
)

// Each output can fail independently when another remains healthy. No retry
// loops are added here; a failed branch is retried on explicit channel restart.
type fanout struct {
	udp     stream.Output
	video   *hls.Session
	lastUDP multicast.Stats
}

func (f *fanout) WritePacket(p []byte) error {
	if f.udp != nil {
		if err := f.udp.WritePacket(p); err != nil {
			_ = f.udp.Close()
			f.lastUDP = *udpStats(f.udp)
			f.udp = nil
		}
	}
	if f.video != nil {
		if err := f.video.WritePacket(p); err != nil {
			_ = f.video.Close()
			f.video = nil
		}
	}
	if f.udp == nil && f.video == nil {
		return errors.New("all configured outputs unavailable")
	}
	return nil
}
func udpStats(out stream.Output) *multicast.Stats {
	if provider, ok := out.(interface{ Snapshot() multicast.Stats }); ok {
		s := provider.Snapshot()
		return &s
	}
	return &multicast.Stats{}
}
func (f *fanout) Snapshot() multicast.Stats {
	if f.udp != nil {
		return *udpStats(f.udp)
	}
	return f.lastUDP
}
func (f *fanout) Close() error {
	if f.udp != nil {
		_ = f.udp.Close()
		f.lastUDP = *udpStats(f.udp)
	}
	if f.video != nil {
		_ = f.video.Close()
	}
	return nil
}
func outputs(udp stream.OutputFactory, manager *hls.Manager, store *catalog.Store) stream.OutputFactory {
	if manager == nil {
		return udp
	}
	return func(ctx context.Context, id string) (stream.Output, error) {
		f := &fanout{}
		if udp != nil {
			out, err := udp(ctx, id)
			if err == nil {
				f.udp = out
			}
		}
		if store.Authorize(id, "hls", time.Now().UTC()) {
			s, err := manager.Open(ctx, id)
			if err == nil {
				f.video = s
			}
		}
		if f.udp == nil && f.video == nil {
			return nil, errors.New("no authorized output available")
		}
		return f, nil
	}
}
