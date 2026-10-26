package live

import (
	"testing"
	"time"
)

func syntheticPacket() []byte {
	p := make([]byte, 188)
	p[0] = 0x47
	p[1] = 0x1f
	p[2] = 0xff
	p[3] = 0x10
	return p
}
func TestHubSharesBoundedHistoryAndDisconnectsSlowConsumers(t *testing.T) {
	hub := NewHub()
	defer hub.Close()
	p := syntheticPacket()
	for i := 0; i < 14; i++ {
		if hub.WritePacket(p) != nil {
			t.Fatal("publish")
		}
	}
	replay, stream, release, e := hub.Subscribe()
	if e != nil || len(replay) != 2 {
		t.Fatal("replay unavailable", e)
	}
	defer release()
	if len(replay[0]) != chunkBytes || replay[0][0] != 0x47 {
		t.Fatal("corrupt replay")
	}
	for i := 0; i < 7; i++ {
		_ = hub.WritePacket(p)
	}
	select {
	case chunk := <-stream:
		if len(chunk) != chunkBytes {
			t.Fatal("invalid live chunk")
		}
	case <-time.After(time.Second):
		t.Fatal("fanout stalled")
	}
	for i := 0; i < 7*40; i++ {
		_ = hub.WritePacket(p)
	}
	if hub.Snapshot().Subscribers != 0 {
		t.Fatal("slow consumer retained")
	}
	for range stream {
	}
	release()
	hub.Close()
	if hub.WritePacket(p) == nil {
		t.Fatal("closed publication accepted")
	}
}
func TestHubRejectsMalformedPacketsAndKeepsCacheBounded(t *testing.T) {
	hub := NewHub()
	defer hub.Close()
	if hub.WritePacket([]byte{0x47}) == nil {
		t.Fatal("invalid framing accepted")
	}
	p := syntheticPacket()
	for i := 0; i < (cacheChunks+20)*7; i++ {
		_ = hub.WritePacket(p)
	}
	replay, _, release, e := hub.Subscribe()
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	if len(replay) != cacheChunks {
		t.Fatal("cache not bounded", len(replay))
	}
	metrics := hub.Snapshot()
	if metrics.Bytes == 0 || metrics.LastPacket.IsZero() || metrics.Bitrate != nil {
		t.Fatal("invalid real/unavailable metrics")
	}
}
