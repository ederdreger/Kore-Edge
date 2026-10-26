package app

import (
	"context"
	"time"

	"kore-edge/internal/catalog"
	"kore-edge/internal/multicast"
	"kore-edge/internal/stream"
)

type guardedUDP struct {
	*multicast.Sender
	store *catalog.Store
	id    string
}

func (g *guardedUDP) WritePacket(p []byte) error {
	if !g.store.Authorize(g.id, "udp", time.Now().UTC()) {
		return stream.ErrDenied
	}
	return g.Sender.WritePacket(p)
}
func multicastOutput(c multicast.Config, store *catalog.Store) stream.OutputFactory {
	if c.Mode != "send" {
		return nil
	}
	targets := map[string]multicast.Target{}
	for _, t := range c.Outputs {
		targets[t.ChannelID] = t
	}
	return func(ctx context.Context, id string) (stream.Output, error) {
		t, ok := targets[id]
		if !ok {
			return nil, nil
		}
		if !store.Authorize(id, "udp", time.Now().UTC()) {
			return nil, stream.ErrDenied
		}
		sender, err := multicast.Open(ctx, t)
		if err != nil {
			return nil, err
		}
		return &guardedUDP{sender, store, id}, nil
	}
}
