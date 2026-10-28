package live

import (
	"context"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type stalledOutput struct {
	entered chan struct{}
	unblock chan struct{}
	once    sync.Once
	closed  sync.Once
	writes  atomic.Int32
}

func newStalledOutput() *stalledOutput {
	return &stalledOutput{entered: make(chan struct{}), unblock: make(chan struct{})}
}
func (o *stalledOutput) WritePacket([]byte) error {
	o.writes.Add(1)
	o.once.Do(func() { close(o.entered) })
	<-o.unblock
	return ErrUnavailable
}
func (o *stalledOutput) Close() error { o.closed.Do(func() { close(o.unblock) }); return nil }

func TestSlowSegmenterDoesNotBlockSourceAndFailsAsAWhole(t *testing.T) {
	sink := newStalledOutput()
	output := isolateOutput(context.Background(), sink)
	defer output.Close()
	if output.WritePacket(syntheticPacket()) != nil {
		t.Fatal("initial packet rejected")
	}
	select {
	case <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("worker did not start")
	}
	done := make(chan bool, 1)
	go func() {
		for i := 0; i < 2048; i++ {
			if output.WritePacket(syntheticPacket()) != nil {
				done <- true
				return
			}
		}
		done <- false
	}()
	select {
	case failed := <-done:
		if !failed {
			t.Fatal("unbounded output queue")
		}
	case <-time.After(time.Second):
		t.Fatal("source blocked by output")
	}
	output.Close()
	output.Close()
	if sink.writes.Load() != 1 {
		t.Fatal("continued feeding a failed segmenter")
	}
}

func TestHLSFailureDoesNotReconnectSharedSource(t *testing.T) {
	catalog := &syntheticCatalog{}
	catalog.allowed.Store(true)
	manager, err := New(context.Background(), filepath.Join(t.TempDir(), "live"), catalog)
	if err != nil {
		t.Fatal(err)
	}
	defer manager.Close()
	id := "synthetic-output-failure"
	if manager.Save(ChannelConfig{ChannelID: id, Protocol: "http_ts"}, Tools{}, 0) != nil || manager.Start(id) != nil {
		t.Fatal("start")
	}
	waitStatus(t, manager, "running")
	manager.mu.Lock()
	j := manager.jobs[id]
	manager.mu.Unlock()
	sink := newStalledOutput()
	sink.Close()
	var releases atomic.Int32
	j.mu.Lock()
	before := j.hub.Snapshot().Bytes
	j.hlsOutput = isolateOutput(j.ctx, sink)
	j.hlsRelease = func() { releases.Add(1) }
	j.status.HLS = "running"
	j.mu.Unlock()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		status := manager.Snapshot().Streams[0]
		if releases.Load() == 1 && status.Metrics.Bytes > before+188*30 {
			if catalog.opens.Load() != 1 || status.Interruptions != 0 || status.State != "running" || status.HLS != "recovering_output" {
				t.Fatal("output failure tore down source", status, catalog.opens.Load())
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("source stopped advancing or output resources leaked")
}
