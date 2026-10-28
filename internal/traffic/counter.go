// Package traffic measures payload bytes without buffering or changing media.
package traffic

import (
	"sync"
	"sync/atomic"
	"time"
)

type Counter struct {
	bytes atomic.Uint64
	mu    sync.Mutex
	at    time.Time
	last  uint64
	rate  *float64
}

func New() *Counter { return &Counter{at: time.Now()} }
func (c *Counter) Add(n int) {
	if n > 0 {
		c.bytes.Add(uint64(n))
	}
}
func (c *Counter) Bytes() uint64  { return c.bytes.Load() }
func (c *Counter) Rate() *float64 { return c.rateAt(time.Now()) }
func (c *Counter) rateAt(now time.Time) *float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elapsed := now.Sub(c.at).Seconds(); elapsed >= 1 {
		total := c.bytes.Load()
		value := float64(total-c.last) * 8 / elapsed
		c.rate = &value
		c.last = total
		c.at = now
	}
	if c.rate == nil {
		return nil
	}
	value := *c.rate
	return &value
}
