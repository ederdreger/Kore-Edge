// Package live connects authorized real ingest to bounded local outputs.
// It does not modify the synthetic catalog or grant network privileges.
package live

import (
	"errors"
	"io"
	"kore-edge/internal/tsmetrics"
	"sync"
	"time"
)

const chunkBytes = 188 * 7
const cacheChunks = 2 * 1024 * 1024 / chunkBytes

var ErrUnavailable = errors.New("live_unavailable")
var ErrDenied = errors.New("live_not_authorized")
var ErrConflict = errors.New("live_configuration_changed")
var ErrInvalid = errors.New("invalid_live_configuration")

type Hub struct {
	mu                          sync.Mutex
	closed                      bool
	pending                     []byte
	ring                        [][]byte
	next                        int
	full                        bool
	subscribers                 map[chan []byte]bool
	bytes                       uint64
	started, timeAt, lastPacket time.Time
	sampleBytes                 uint64
	bitrate                     *float64
	window                      float64
	analyzer                    *tsmetrics.Analyzer
}
type Metrics struct {
	Bytes         uint64             `json:"bytes"`
	Bitrate       *float64           `json:"bits_per_second"`
	LastPacket    time.Time          `json:"last_packet_at"`
	WindowSeconds float64            `json:"window_seconds"`
	Transport     tsmetrics.Snapshot `json:"transport"`
	Subscribers   int                `json:"subscribers"`
}

func NewHub() *Hub {
	now := time.Now().UTC()
	return &Hub{ring: make([][]byte, cacheChunks), subscribers: map[chan []byte]bool{}, started: now, timeAt: now, analyzer: tsmetrics.New(now)}
}
func (h *Hub) WritePacket(packet []byte) error {
	if len(packet) != 188 || packet[0] != 0x47 {
		return ErrInvalid
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return io.ErrClosedPipe
	}
	now := time.Now().UTC()
	h.analyzer.Observe(packet, now)
	h.bytes += 188
	h.lastPacket = now
	if duration := now.Sub(h.timeAt).Seconds(); duration >= 1 {
		rate := 8 * float64(h.bytes-h.sampleBytes) / duration
		h.bitrate = &rate
		h.window = duration
		h.timeAt = now
		h.sampleBytes = h.bytes
	}
	h.pending = append(h.pending, packet...)
	if len(h.pending) < chunkBytes {
		return nil
	}
	chunk := h.pending
	h.pending = nil
	h.ring[h.next] = chunk
	h.next = (h.next + 1) % len(h.ring)
	if h.next == 0 {
		h.full = true
	}
	for subscriber := range h.subscribers {
		select {
		case subscriber <- chunk:
		default:
			close(subscriber)
			delete(h.subscribers, subscriber)
		}
	}
	return nil
}
func (h *Hub) Subscribe() ([][]byte, <-chan []byte, func(), error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.subscribers) >= 32 {
		return nil, nil, nil, ErrUnavailable
	}
	replay := make([][]byte, 0, len(h.ring))
	start, end := 0, h.next
	if h.full {
		start = h.next
		end = start + len(h.ring)
	}
	for index := start; index < end; index++ {
		replay = append(replay, h.ring[index%len(h.ring)])
	}
	subscriber := make(chan []byte, 32)
	h.subscribers[subscriber] = true
	var once sync.Once
	release := func() {
		once.Do(func() {
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.subscribers[subscriber] {
				close(subscriber)
				delete(h.subscribers, subscriber)
			}
		})
	}
	return replay, subscriber, release, nil
}
func (h *Hub) Snapshot() Metrics {
	h.mu.Lock()
	defer h.mu.Unlock()
	var rate *float64
	if h.bitrate != nil && time.Since(h.lastPacket) < 3*time.Second {
		value := *h.bitrate
		rate = &value
	}
	window := h.window
	if rate == nil {
		window = 0
	}
	return Metrics{Bytes: h.bytes, Bitrate: rate, LastPacket: h.lastPacket, WindowSeconds: window, Transport: h.analyzer.Snapshot(), Subscribers: len(h.subscribers)}
}
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for subscriber := range h.subscribers {
		close(subscriber)
	}
	h.subscribers = map[chan []byte]bool{}
	h.ring = nil
	h.pending = nil
}
