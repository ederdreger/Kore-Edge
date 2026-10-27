package live

import (
	"context"
	"io"
	"kore-edge/internal/edgecatalog"
	"kore-edge/internal/hls"
	"kore-edge/internal/multicast"
	"sync"
	"time"
)

type SourceOpener func(context.Context, ChannelConfig, Tools) (io.ReadCloser, error)
type Catalog interface {
	Snapshot() edgecatalog.Snapshot
	Authorized(string, string) bool
	OutputAuthorized(string, string) bool
	OpenHTTP(context.Context, string) (io.ReadCloser, error)
	OpenHLS(context.Context, string, string) (io.ReadCloser, error)
	OpenSRT(context.Context, string) (io.ReadCloser, error)
}
type Output interface {
	WritePacket([]byte) error
	Close() error
}
type Status struct {
	ChannelID     string    `json:"channel_id"`
	Protocol      string    `json:"protocol"`
	State         string    `json:"state"`
	Error         string    `json:"error,omitempty"`
	StartedAt     time.Time `json:"started_at"`
	FirstPacketAt time.Time `json:"first_packet_at"`
	StartupMS     *int64    `json:"startup_ms"`
	Attempts      int       `json:"attempts"`
	Metrics       Metrics   `json:"metrics"`
	UDP           string    `json:"udp_state"`
	HLS           string    `json:"hls_state"`
}
type job struct {
	mu          sync.Mutex
	config      ChannelConfig
	tools       Tools
	status      Status
	hub         *Hub
	ctx         context.Context
	cancel      context.CancelFunc
	done        chan struct{}
	udp         Output
	hlsManager  *hls.Manager
	hlsOutput   Output
	hlsStarting bool
	hlsWG       sync.WaitGroup
}
type Manager struct {
	mu           sync.Mutex
	ctx          context.Context
	cancel       context.CancelFunc
	store        *configStore
	catalog      Catalog
	jobs         map[string]*job
	open         SourceOpener
	interfaces   func() ([]multicast.Interface, error)
	udpOpen      func(context.Context, multicast.Target, []multicast.Interface) (*multicast.Sender, error)
	closed       bool
	previewSlots chan struct{}
}
type Snapshot struct {
	ManualRevision uint64               `json:"manual_revision"`
	Configuration  Configuration        `json:"configuration"`
	Catalog        edgecatalog.Snapshot `json:"catalog"`
	Streams        []Status             `json:"streams"`
	MaxActive      int                  `json:"max_active"`
}

func New(ctx context.Context, directory string, catalog Catalog) (*Manager, error) {
	if catalog == nil {
		return nil, ErrUnavailable
	}
	store, e := loadConfig(directory)
	if e != nil {
		return nil, e
	}
	child, cancel := context.WithCancel(ctx)
	manager := &Manager{ctx: child, cancel: cancel, store: store, catalog: catalog, jobs: map[string]*job{}, interfaces: multicast.Interfaces, udpOpen: multicast.OpenAuthorized, previewSlots: make(chan struct{}, 32)}
	manager.open = func(ctx context.Context, config ChannelConfig, tools Tools) (io.ReadCloser, error) {
		switch config.Protocol {
		case "http_ts":
			return catalog.OpenHTTP(ctx, config.ChannelID)
		case "srt":
			return catalog.OpenSRT(ctx, config.ChannelID)
		case "hls":
			if tools.FFmpeg == "" {
				return nil, ErrUnavailable
			}
			return catalog.OpenHLS(ctx, config.ChannelID, tools.FFmpeg)
		case "udp":
			if receiver, ok := catalog.(interface {
				OpenUDP(context.Context, string) (io.ReadCloser, error)
			}); ok {
				return receiver.OpenUDP(ctx, config.ChannelID)
			}
			return nil, ErrUnavailable
		default:
			return nil, ErrUnavailable
		}
	}
	return manager, nil
}
func (m *Manager) Snapshot() Snapshot {
	m.mu.Lock()
	jobs := make([]*job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.Unlock()
	streams := make([]Status, 0, len(jobs))
	for _, j := range jobs {
		j.mu.Lock()
		status := j.status
		hub := j.hub
		segmenter := j.hlsManager
		j.mu.Unlock()
		if status.HLS == "running" && segmenter != nil && !segmenter.Ready(status.ChannelID) {
			status.HLS = "warming"
		}
		if hub != nil {
			status.Metrics = hub.Snapshot()
		}
		if status.StartupMS != nil {
			value := *status.StartupMS
			status.StartupMS = &value
		}
		streams = append(streams, status)
	}
	view := Snapshot{Configuration: m.store.snapshot(), Catalog: m.catalog.Snapshot(), Streams: streams, MaxActive: 16}
	if local, ok := m.catalog.(interface{ Revision() uint64 }); ok {
		view.ManualRevision = local.Revision()
	}
	return view
}

func (m *Manager) IsActive(id string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.jobs[id]; j != nil {
		select {
		case <-j.done:
		default:
			return true
		}
	}
	return false
}

func (m *Manager) Forget(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if j := m.jobs[id]; j != nil {
		select {
		case <-j.done:
		default:
			return ErrConflict
		}
	}
	current := m.store.snapshot()
	expected := current.Generation
	next := current
	next.Channels = []ChannelConfig{}
	for _, c := range current.Channels {
		if c.ChannelID != id {
			next.Channels = append(next.Channels, c)
		}
	}
	if len(next.Channels) == len(current.Channels) {
		return nil
	}
	return m.store.replace(next, expected)
}
func (m *Manager) Save(config ChannelConfig, tools Tools, expected uint64) error {
	if !m.catalog.Authorized(config.ChannelID, config.Protocol) {
		return ErrDenied
	}
	if config.UDP != nil && !m.catalog.OutputAuthorized(config.ChannelID, "udp") || config.HLS && !m.catalog.OutputAuthorized(config.ChannelID, "hls") {
		return ErrDenied
	}
	if (config.Protocol == "hls" || config.HLS) && tools.FFmpeg == "" || config.HLS && tools.FFprobe == "" {
		return ErrUnavailable
	}
	if config.UDP != nil {
		all, e := m.interfaces()
		if e != nil {
			return ErrUnavailable
		}
		if _, e = multicast.DryRun(*config.UDP, all); e != nil {
			return ErrInvalid
		}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrUnavailable
	}
	current := m.store.snapshot()
	if expected != current.Generation {
		return ErrConflict
	}
	for _, j := range m.jobs {
		select {
		case <-j.done:
		default:
			if j.config.ChannelID == config.ChannelID || current.Tools != tools {
				return ErrConflict
			}
		}
	}
	current.Tools = tools
	found := false
	for i, entry := range current.Channels {
		if entry.ChannelID == config.ChannelID {
			current.Channels[i] = config
			found = true
			break
		}
	}
	if !found {
		current.Channels = append(current.Channels, config)
	}
	return m.store.replace(current, expected)
}
func (m *Manager) PlanUDP(target multicast.Target) (multicast.Plan, error) {
	if !m.catalog.OutputAuthorized(target.ChannelID, "udp") {
		return multicast.Plan{}, ErrDenied
	}
	all, e := m.interfaces()
	if e != nil {
		return multicast.Plan{}, ErrUnavailable
	}
	return multicast.DryRun(target, all)
}
func (m *Manager) Start(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.startLocked(id)
}
func (m *Manager) StartReviewed(id string, expected uint64) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store.snapshot().Generation != expected {
		return ErrConflict
	}
	return m.startLocked(id)
}
func (m *Manager) startLocked(id string) error {
	if m.closed || m.ctx.Err() != nil {
		return ErrUnavailable
	}
	config := m.store.snapshot()
	var selected *ChannelConfig
	for _, entry := range config.Channels {
		if entry.ChannelID == id {
			copy := entry
			selected = &copy
			break
		}
	}
	if selected == nil {
		return ErrInvalid
	}
	if !m.catalog.Authorized(id, selected.Protocol) {
		return ErrDenied
	}
	count := 0
	for key, j := range m.jobs {
		select {
		case <-j.done:
		default:
			if key == id {
				return nil
			}
			count++
		}
	}
	if count >= 16 || len(m.jobs) >= 512 && m.jobs[id] == nil {
		return ErrConflict
	}
	child, cancel := context.WithCancel(m.ctx)
	j := &job{config: *selected, tools: config.Tools, ctx: child, cancel: cancel, done: make(chan struct{}), hub: NewHub(), status: Status{ChannelID: id, Protocol: selected.Protocol, State: "starting", StartedAt: time.Now().UTC(), UDP: "disabled", HLS: "disabled"}}
	m.jobs[id] = j
	go m.run(j)
	return nil
}
func (m *Manager) Stop(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	j := m.jobs[id]
	if j != nil {
		j.cancel()
		select {
		case <-j.done:
		case <-time.After(3 * time.Second):
			return ErrUnavailable
		}
	}
	current := m.store.snapshot()
	for i, entry := range current.Channels {
		if entry.ChannelID == id && entry.Resume {
			current.Channels[i].Resume = false
			return m.store.replace(current, current.Generation)
		}
	}
	return nil
}
func (m *Manager) Resume() {
	for _, entry := range m.store.snapshot().Channels {
		if entry.Resume {
			_ = m.Start(entry.ChannelID)
		}
	}
}
func (m *Manager) Close() {
	m.mu.Lock()
	m.closed = true
	m.cancel()
	jobs := make([]*job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.Unlock()
	for _, j := range jobs {
		<-j.done
	}
}
func (m *Manager) run(j *job) {
	defer close(j.done)
	defer j.cancel()
	defer func() { j.mu.Lock(); hub := j.hub; j.mu.Unlock(); hub.Close() }()
	for attempt := 0; attempt < 4; attempt++ {
		if j.ctx.Err() != nil {
			j.setState("stopped", "")
			return
		}
		if !m.catalog.Authorized(j.config.ChannelID, j.config.Protocol) {
			j.setState("unauthorized", "source_grant_revoked")
			return
		}
		j.mu.Lock()
		j.status.Attempts = attempt + 1
		j.status.State = "starting"
		j.status.Error = ""
		j.status.FirstPacketAt = time.Time{}
		j.status.StartupMS = nil
		old := j.hub
		j.hub = NewHub()
		j.mu.Unlock()
		old.Close()
		e := m.attempt(j)
		if j.ctx.Err() != nil {
			j.setState("stopped", "")
			return
		}
		if e == ErrDenied {
			j.setState("unauthorized", "source_grant_revoked")
			return
		}
		if attempt == 3 {
			j.setState("failed", "source_unavailable_or_invalid_ts")
			return
		}
		j.setState("backoff", "source_interrupted")
		timer := time.NewTimer(time.Duration(1<<attempt) * 500 * time.Millisecond)
		select {
		case <-j.ctx.Done():
			timer.Stop()
			j.setState("stopped", "")
			return
		case <-timer.C:
		}
	}
}
func (j *job) setState(state, message string) {
	j.mu.Lock()
	j.status.State = state
	j.status.Error = message
	j.mu.Unlock()
}
