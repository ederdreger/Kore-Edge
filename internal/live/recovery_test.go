package live

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

type recoveryCatalog struct {
	syntheticCatalog
	protocol string
}

func (c *recoveryCatalog) Authorized(id, p string) bool { return c.allowed.Load() && p == c.protocol }

func TestReceptionRecoversBeyondFourInterruptionsAndHonorsManualStop(t *testing.T) {
	for _, protocol := range []string{"http_ts", "srt"} {
		t.Run(protocol, func(t *testing.T) {
			c := &recoveryCatalog{protocol: protocol}
			c.allowed.Store(true)
			m, e := New(context.Background(), filepath.Join(t.TempDir(), "live"), c)
			if e != nil {
				t.Fatal(e)
			}
			defer m.Close()
			m.retryDelay = func(int) time.Duration { return time.Millisecond }
			var opens atomic.Int32
			m.open = func(ctx context.Context, _ ChannelConfig, _ Tools) (io.ReadCloser, error) {
				n := opens.Add(1)
				if n < 7 {
					return io.NopCloser(bytes.NewReader(bytes.Repeat(syntheticPacket(), 21))), nil
				}
				r, w := io.Pipe()
				go func() {
					defer w.Close()
					for {
						select {
						case <-ctx.Done():
							return
						case <-time.After(time.Millisecond):
							if _, e := w.Write(bytes.Repeat(syntheticPacket(), 7)); e != nil {
								return
							}
						}
					}
				}()
				return r, nil
			}
			if e := m.Save(ChannelConfig{ChannelID: "synthetic-recovery", Protocol: protocol}, Tools{}, 0); e != nil {
				t.Fatal(e)
			}
			if e := m.Start("synthetic-recovery"); e != nil {
				t.Fatal(e)
			}
			deadline := time.Now().Add(3 * time.Second)
			for opens.Load() < 7 && time.Now().Before(deadline) {
				time.Sleep(time.Millisecond)
			}
			waitStatus(t, m, "running")
			view := m.Snapshot()
			if opens.Load() < 7 || view.Streams[0].Interruptions < 6 || view.Streams[0].LastFailureAt.IsZero() {
				t.Fatal("recovery stopped early", opens.Load(), view.Streams)
			}
			if e := m.Stop("synthetic-recovery"); e != nil {
				t.Fatal(e)
			}
			before := opens.Load()
			time.Sleep(20 * time.Millisecond)
			if opens.Load() != before {
				t.Fatal("manual stop ignored")
			}
		})
	}
}

func TestRecoveryBackoffIsCapped(t *testing.T) {
	for _, n := range []int{0, 1, 10, 1000000} {
		d := recoveryDelay(n)
		if d < 500*time.Millisecond || d > 30*time.Second {
			t.Fatal("unbounded retry interval", n, d)
		}
	}
}

func TestHubToleratesHTTPJitterWithoutRemovingConsumer(t *testing.T) {
	h := NewHub()
	defer h.Close()
	_, stream, release, e := h.Subscribe()
	if e != nil {
		t.Fatal(e)
	}
	defer release()
	for i := 0; i < 7*200; i++ {
		if e := h.WritePacket(syntheticPacket()); e != nil {
			t.Fatal(e)
		}
	}
	if h.Snapshot().Subscribers != 1 {
		t.Fatal("brief HTTP jitter disconnected player")
	}
	for i := 0; i < 200; i++ {
		if chunk := <-stream; len(chunk) != chunkBytes {
			t.Fatal("broken transport framing")
		}
	}
}
