// Package stream supervises synthetic sources with optional bounded outputs.
package stream

import (
	"context"
	"errors"
	"kore-edge/internal/multicast"
	"kore-edge/internal/tsmetrics"
	"sort"
	"sync"
	"time"
)

var ErrDenied = errors.New("channel not authorized")
var ErrLimit = errors.New("stream resource limit")
var ErrClosed = errors.New("engine closed")

type Authorizer func(string, time.Time) bool
type Output interface {
	WritePacket([]byte) error
	Close() error
}

func udpSnapshot(out Output) *multicast.Stats {
	if provider, ok := out.(interface{ Snapshot() multicast.Stats }); ok {
		stats := provider.Snapshot()
		if stats.State != "" {
			return &stats
		}
	}
	return nil
}

type OutputFactory func(context.Context, string) (Output, error)
type Policy struct {
	MaxActive, MaxHistory, MaxRestarts int
	Backoff, StallTimeout, Poll        time.Duration
}

func DefaultPolicy() Policy {
	return Policy{4, 64, 3, 250 * time.Millisecond, 2 * time.Second, 100 * time.Millisecond}
}
func (p Policy) valid() bool {
	return p.MaxActive > 0 && p.MaxActive <= 16 && p.MaxHistory >= p.MaxActive && p.MaxHistory <= 128 && p.MaxRestarts >= 0 && p.MaxRestarts <= 5 && p.Backoff > 0 && p.Backoff <= 30*time.Second && p.StallTimeout > p.Poll && p.Poll > 0
}

// retryDelay is capped before multiplication; malformed attempts cannot overflow.
func (p Policy) retryDelay(attempt int) time.Duration {
	if attempt < 0 {
		return 0
	}
	delay := p.Backoff
	for i := 0; i < attempt; i++ {
		if delay >= 15*time.Second {
			return 30 * time.Second
		}
		delay *= 2
	}
	if delay > 30*time.Second {
		return 30 * time.Second
	}
	return delay
}

type Rate struct {
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
	State         string    `json:"state"`
	BitsPerSecond *float64  `json:"bits_per_second"`
	WindowSeconds float64   `json:"window_seconds"`
}
type Status struct {
	Transport       tsmetrics.Snapshot `json:"transport"`
	UDP             *multicast.Stats   `json:"udp,omitempty"`
	ChannelID       string             `json:"channel_id"`
	State           string             `json:"state"`
	Source          string             `json:"source"`
	Timestamp       time.Time          `json:"timestamp"`
	StartedAt       time.Time          `json:"started_at"`
	LastPacketAt    *time.Time         `json:"last_packet_at"`
	Attempts        int                `json:"attempts"`
	Restarts        int                `json:"restarts"`
	Bytes           uint64             `json:"bytes"`
	Packets         uint64             `json:"packets"`
	SyncErrors      uint64             `json:"sync_errors"`
	TransportErrors uint64             `json:"transport_errors"`
	Bitrate         Rate               `json:"bitrate"`
	MediaQuality    string             `json:"media_quality"`
}
type job struct {
	mu           sync.Mutex
	status       Status
	cancel       context.CancelFunc
	done         chan struct{}
	lastProgress time.Time
	sampleAt     time.Time
	sampleBytes  uint64
}
type Engine struct {
	mu      sync.Mutex
	ctx     context.Context
	cancel  context.CancelFunc
	factory Factory
	allow   Authorizer
	policy  Policy
	jobs    map[string]*job
	closed  bool
	output  OutputFactory
}

func New(ctx context.Context, factory Factory, allow Authorizer, policy Policy) (*Engine, error) {
	return NewWithOutput(ctx, factory, allow, policy, nil)
}
func NewWithOutput(ctx context.Context, factory Factory, allow Authorizer, policy Policy, output OutputFactory) (*Engine, error) {
	if factory == nil || allow == nil || !policy.valid() {
		return nil, errors.New("invalid engine options")
	}
	child, cancel := context.WithCancel(ctx)
	return &Engine{ctx: child, cancel: cancel, factory: factory, allow: allow, policy: policy, jobs: map[string]*job{}, output: output}, nil
}
func (e *Engine) Start(id string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.ctx.Err() != nil {
		return ErrClosed
	}
	now := time.Now().UTC()
	if !e.allow(id, now) {
		return ErrDenied
	}
	if old := e.jobs[id]; old != nil {
		select {
		case <-old.done:
		default:
			return nil
		}
	}
	active := 0
	for _, j := range e.jobs {
		select {
		case <-j.done:
		default:
			active++
		}
	}
	if active >= e.policy.MaxActive {
		return ErrLimit
	}
	if e.jobs[id] == nil && len(e.jobs) >= e.policy.MaxHistory {
		return ErrLimit
	}
	ctx, cancel := context.WithCancel(e.ctx)
	j := &job{cancel: cancel, done: make(chan struct{}), sampleAt: now, status: Status{ChannelID: id, State: "starting", Source: "synthetic:null-ts", StartedAt: now, Bitrate: Rate{State: "unavailable"}, MediaQuality: "unavailable: null packets contain no audio/video, PCR or service tables"}}
	e.jobs[id] = j
	go e.run(ctx, j)
	return nil
}
func (e *Engine) Stop(id string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	j := e.jobs[id]
	if j == nil {
		return false
	}
	j.cancel()
	return true
}
func (e *Engine) Close() {
	e.mu.Lock()
	e.closed = true
	e.cancel()
	jobs := make([]*job, 0, len(e.jobs))
	for _, j := range e.jobs {
		jobs = append(jobs, j)
	}
	e.mu.Unlock()
	for _, j := range jobs {
		<-j.done
	}
}
func (e *Engine) Snapshot() []Status {
	e.mu.Lock()
	defer e.mu.Unlock()
	result := make([]Status, 0, len(e.jobs))
	now := time.Now().UTC()
	for _, j := range e.jobs {
		j.mu.Lock()
		s := j.status
		s.Transport = s.Transport.Clone()
		if s.UDP != nil {
			stats := *s.UDP
			s.UDP = &stats
		}
		s.Timestamp = now
		if s.LastPacketAt != nil {
			last := *s.LastPacketAt
			s.LastPacketAt = &last
		}
		if s.Bitrate.BitsPerSecond != nil {
			rate := *s.Bitrate.BitsPerSecond
			s.Bitrate.BitsPerSecond = &rate
		}
		j.mu.Unlock()
		result = append(result, s)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ChannelID < result[j].ChannelID })
	return result
}
func (j *job) setState(s string) {
	j.mu.Lock()
	j.status.State = s
	if s != "running" {
		j.status.Bitrate = Rate{State: "unavailable"}
	}
	j.mu.Unlock()
}
func (e *Engine) run(ctx context.Context, j *job) {
	defer close(j.done)
	defer j.cancel()
	for attempt := 0; attempt <= e.policy.MaxRestarts; attempt++ {
		if ctx.Err() != nil {
			j.setState("stopped")
			return
		}
		if !e.allow(j.status.ChannelID, time.Now().UTC()) {
			j.setState("unauthorized")
			return
		}
		j.mu.Lock()
		j.status.Attempts++
		j.status.Restarts = attempt
		j.status.State = "starting"
		j.lastProgress = time.Now().UTC()
		j.sampleAt = j.lastProgress
		j.sampleBytes = j.status.Bytes
		j.status.Bitrate = Rate{State: "unavailable"}
		j.status.Transport = tsmetrics.Snapshot{CCState: "unavailable", PCRState: "unavailable"}
		j.status.UDP = nil
		j.mu.Unlock()
		e2 := e.runAttempt(ctx, j)
		if ctx.Err() != nil {
			j.setState("stopped")
			return
		}
		if errors.Is(e2, ErrDenied) {
			j.setState("unauthorized")
			return
		}
		if attempt == e.policy.MaxRestarts {
			j.setState("failed")
			return
		}
		j.setState("backoff")
		timer := time.NewTimer(e.policy.retryDelay(attempt))
		select {
		case <-ctx.Done():
			timer.Stop()
			j.setState("stopped")
			return
		case <-timer.C:
		}
	}
}
func (e *Engine) runAttempt(ctx context.Context, j *job) error {
	var out Output
	if e.output != nil {
		var err error
		out, err = e.output(ctx, j.status.ChannelID)
		if err != nil {
			return err
		}
	}
	if out != nil {
		defer func() { _ = out.Close(); stats := udpSnapshot(out); j.mu.Lock(); j.status.UDP = stats; j.mu.Unlock() }()
	}
	p, err := e.factory(ctx)
	if err != nil {
		return err
	}
	if p.Source != "" {
		j.mu.Lock()
		j.status.Source = p.Source
		j.status.MediaQuality = "unavailable: full media quality and calibrated PCR accuracy are not measured"
		j.mu.Unlock()
	}
	return e.attempt(ctx, j, p, out)
}
func (e *Engine) attempt(ctx context.Context, j *job, p *Process, out Output) error {
	defer p.Output.Close()
	done := make(chan error, 1)
	go func() {
		analyzer := tsmetrics.New(time.Now().UTC())
		send := func(packet []byte) error {
			analyzer.Observe(packet, time.Now().UTC())
			stats := analyzer.Snapshot()
			j.mu.Lock()
			j.status.Transport = stats
			j.mu.Unlock()
			if out != nil {
				err := out.WritePacket(packet)
				stats := udpSnapshot(out)
				j.mu.Lock()
				j.status.UDP = stats
				j.mu.Unlock()
				return err
			}
			return nil
		}
		readErr := ingestTo(p.Output, func(b, packets, syncErrors, tei uint64) {
			j.mu.Lock()
			defer j.mu.Unlock()
			j.status.Bytes += b
			j.status.Packets += packets
			j.status.SyncErrors += syncErrors
			j.status.TransportErrors += tei
			if packets > 0 {
				now := time.Now().UTC()
				j.lastProgress = now
				j.status.LastPacketAt = &now
				j.status.State = "running"
			}
		}, send)
		if readErr != nil {
			_ = p.Kill()
		}
		waitErr := p.Wait()
		if readErr != nil {
			done <- readErr
		} else {
			done <- waitErr
		}
	}()
	ticker := time.NewTicker(e.policy.Poll)
	defer ticker.Stop()
	stop := func(err error) error { _ = p.Kill(); _ = p.Output.Close(); <-done; return err }
	for {
		select {
		case <-ctx.Done():
			return stop(ctx.Err())
		case err := <-done:
			return err
		case <-ticker.C:
			now := time.Now().UTC()
			if !e.allow(j.status.ChannelID, now) {
				return stop(ErrDenied)
			}
			j.mu.Lock()
			stalled := now.Sub(j.lastProgress) >= e.policy.StallTimeout
			window := now.Sub(j.sampleAt).Seconds()
			if window > 0 && j.status.Packets > 0 {
				rate := float64(j.status.Bytes-j.sampleBytes) * 8 / window
				j.status.Bitrate = Rate{State: "measured_synthetic", BitsPerSecond: &rate, WindowSeconds: window, WindowStart: j.sampleAt, WindowEnd: now}
				j.sampleAt = now
				j.sampleBytes = j.status.Bytes
			}
			j.mu.Unlock()
			if stalled {
				return stop(errors.New("source stalled"))
			}
		}
	}
}
