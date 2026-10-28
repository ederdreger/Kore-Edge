package live

import (
	"io"
	"kore-edge/internal/traffic"
	"net/http"
	"sync"
	"sync/atomic"
)

type DeliverySnapshot struct {
	Bytes   uint64   `json:"http_bytes"`
	Rate    *float64 `json:"http_bits_per_second"`
	Clients int32    `json:"ts_connections"`
	Scope   string   `json:"scope"`
}
type deliveryCounter struct {
	traffic  *traffic.Counter
	clients  atomic.Int32
	requests atomic.Int32
}
type deliveryCounters struct {
	mu      sync.Mutex
	entries map[string]*deliveryCounter
}

func (m *Manager) deliveryFor(id string) *deliveryCounter {
	d := &m.delivery
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.entries == nil {
		d.entries = map[string]*deliveryCounter{}
	}
	if c := d.entries[id]; c != nil {
		return c
	}
	if len(d.entries) >= 512 {
		for key, c := range d.entries {
			if c.requests.Load() == 0 {
				delete(d.entries, key)
				break
			}
		}
	}
	if len(d.entries) >= 512 {
		return nil
	}
	c := &deliveryCounter{traffic: traffic.New()}
	d.entries[id] = c
	return c
}

type deliveryWriter struct {
	http.ResponseWriter
	counter *deliveryCounter
}

func (w *deliveryWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
func (w *deliveryWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	w.counter.traffic.Add(n)
	return n, err
}
func (w *deliveryWriter) ReadFrom(r io.Reader) (int64, error) {
	if reader, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		n, err := reader.ReadFrom(r)
		w.counter.traffic.Add(int(n))
		return n, err
	}
	return io.Copy(struct{ io.Writer }{w}, r)
}
func (m *Manager) observeDelivery(w http.ResponseWriter, id string, ts bool) (http.ResponseWriter, func()) {
	c := m.deliveryFor(id)
	if c == nil {
		return w, func() {}
	}
	c.requests.Add(1)
	if ts {
		c.clients.Add(1)
	}
	var once sync.Once
	return &deliveryWriter{w, c}, func() {
		once.Do(func() {
			c.requests.Add(-1)
			if ts {
				c.clients.Add(-1)
			}
		})
	}
}
func (m *Manager) deliverySnapshot(id string) *DeliverySnapshot {
	c := m.deliveryFor(id)
	if c == nil {
		return nil
	}
	return &DeliverySnapshot{c.traffic.Bytes(), c.traffic.Rate(), c.clients.Load(), "http_ts_hls"}
}
