// Package monitoring samples existing observations, never controls the host.
package monitoring

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"kore-edge/internal/diagnostics"
	"kore-edge/internal/stream"
)

type Measurement struct {
	Name        string    `json:"name"`
	ChannelID   string    `json:"channel_id,omitempty"`
	Interface   string    `json:"interface,omitempty"`
	State       string    `json:"state"`
	Source      string    `json:"source"`
	Origin      string    `json:"origin"`
	Unit        string    `json:"unit"`
	Timestamp   time.Time `json:"timestamp"`
	WindowStart time.Time `json:"window_start"`
	WindowEnd   time.Time `json:"window_end"`
	Value       *float64  `json:"value"`
	Reason      string    `json:"reason,omitempty"`
}
type Channel struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Source string `json:"source"`
	Origin string `json:"origin"`
}
type Snapshot struct {
	Timestamp             time.Time     `json:"timestamp"`
	State                 string        `json:"state"`
	StreamsState          string        `json:"streams_state"`
	SampleIntervalSeconds float64       `json:"sample_interval_seconds"`
	Channels              []Channel     `json:"channels"`
	Measurements          []Measurement `json:"measurements"`
	Alerts                []Alert       `json:"alerts"`
	RecentAlerts          []Alert       `json:"recent_alerts"`
}
type networkSample struct {
	at     time.Time
	rx, tx uint64
}
type Service struct {
	mu              sync.RWMutex
	streams         func() []stream.Status
	network         func() diagnostics.Metric
	current         Snapshot
	previousNIC     map[string]networkSample
	previousStreams map[string]stream.Status
	active          map[string]Alert
	recent          []Alert
}

func New(streams func() []stream.Status, network func() diagnostics.Metric) *Service {
	return &Service{streams: streams, network: network, current: Snapshot{State: "collecting", SampleIntervalSeconds: 1, Channels: []Channel{}, Measurements: []Measurement{}, Alerts: []Alert{}, RecentAlerts: []Alert{}}, previousNIC: map[string]networkSample{}, previousStreams: map[string]stream.Status{}, active: map[string]Alert{}}
}
func (s *Service) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		var channels []stream.Status
		if s.streams != nil {
			channels = s.streams()
		}
		network := diagnostics.Metric{State: "unavailable", Source: "/proc/net/dev", Reason: "collector unavailable", Timestamp: time.Now().UTC()}
		if s.network != nil {
			network = s.network()
		}
		s.Sample(time.Now().UTC(), channels, network)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
func (s *Service) Snapshot(now time.Time) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := s.current
	out.Channels = append([]Channel{}, out.Channels...)
	out.Measurements = append([]Measurement{}, out.Measurements...)
	for i := range out.Measurements {
		m := &out.Measurements[i]
		if m.Value != nil {
			v := *m.Value
			m.Value = &v
		}
		if now.Sub(out.Timestamp) > 3*time.Second && m.State == "measured" {
			m.State = "unavailable"
			m.Value = nil
			m.Reason = "sampler stale"
		}
	}
	out.Alerts = append([]Alert{}, out.Alerts...)
	out.RecentAlerts = append([]Alert{}, out.RecentAlerts...)
	if !out.Timestamp.IsZero() && now.Sub(out.Timestamp) > 3*time.Second {
		out.State = "stale"
	}
	return out
}
func metric(name, channel, iface, source, origin, unit string, start, end time.Time, value *float64, reason string) Measurement {
	state := "measured"
	if value == nil {
		state = "unavailable"
	}
	return Measurement{Name: name, ChannelID: channel, Interface: iface, Source: source, Origin: origin, Unit: unit, State: state, Timestamp: end, WindowStart: start, WindowEnd: end, Value: value, Reason: reason}
}
func number(v float64) *float64 { return &v }
func delta(current, previous uint64) uint64 {
	if current < previous {
		return current
	}
	return current - previous
}

// Sample is deterministic for supplied clock/fixtures. Production calls it once
// per second with read-only collectors; no unbounded history is retained.
func (s *Service) Sample(now time.Time, channels []stream.Status, network diagnostics.Metric) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.current.Timestamp.IsZero() && (now.Before(s.current.Timestamp) || now.Sub(s.current.Timestamp) > 3*time.Second) {
		for id, alert := range s.active {
			if alert.State == "pending" {
				alert.Since = now
				s.active[id] = alert
			}
		}
	}
	if len(channels) > 64 {
		channels = channels[:64]
	}
	result := Snapshot{Timestamp: now, State: "measured", SampleIntervalSeconds: 1, StreamsState: "enabled", Channels: []Channel{}, Measurements: []Measurement{}, Alerts: []Alert{}, RecentAlerts: []Alert{}}
	if s.streams == nil {
		result.StreamsState = "disabled"
	}
	conditions := []condition{}
	previous := map[string]stream.Status{}
	for _, c := range channels {
		origin := "unknown"
		if strings.HasPrefix(c.Source, "synthetic:") {
			origin = "synthetic"
		}
		result.Channels = append(result.Channels, Channel{c.ChannelID, c.State, c.Source, origin})
		add := func(name, unit string, start, end time.Time, value *float64, reason string) {
			result.Measurements = append(result.Measurements, metric(name, c.ChannelID, "", c.Source, origin, unit, start, end, value, reason))
		}
		available := c.State == "running" && c.LastPacketAt != nil && now.Sub(*c.LastPacketAt) >= 0 && now.Sub(*c.LastPacketAt) <= 2*time.Second
		up := 0.0
		if available {
			up = 1
		}
		add("stream_ingest_available", "boolean", now, now, number(up), "")
		bitrate := c.Bitrate.BitsPerSecond
		reason := ""
		if c.State != "running" || c.Bitrate.WindowStart.IsZero() || now.Sub(c.Bitrate.WindowEnd) > 2*time.Second || c.Bitrate.WindowEnd.After(now) {
			bitrate = nil
			reason = "no fresh bitrate window"
		}
		add("stream_ingest_bitrate_bits_per_second", "bits_per_second", c.Bitrate.WindowStart, c.Bitrate.WindowEnd, bitrate, reason)
		add("stream_ingest_bytes_total", "bytes", c.StartedAt, now, number(float64(c.Bytes)), "")
		add("stream_sync_errors_total", "errors", c.StartedAt, now, number(float64(c.SyncErrors)), "")
		add("stream_tei_packets_total", "packets", c.StartedAt, now, number(float64(c.TransportErrors)), "")
		var cc *float64
		reason = "no non-null PID observed"
		if c.Transport.CCState == "measured" {
			cc = number(float64(c.Transport.CCErrors))
			reason = ""
		}
		add("stream_cc_errors_total", "errors", c.Transport.WindowStart, c.Transport.WindowEnd, cc, reason)
		var pcrCount *float64
		if !c.Transport.WindowStart.IsZero() {
			pcrCount = number(float64(c.Transport.PCRCount))
		}
		add("stream_pcr_observations_total", "observations", c.Transport.WindowStart, c.Transport.WindowEnd, pcrCount, "requires sampled TS")
		interval := c.Transport.PCRIntervalSeconds
		reason = "PCR pair unavailable"
		if c.Transport.PCRObservedAt == nil || c.Transport.PCRObservedAt.After(now) || now.Sub(*c.Transport.PCRObservedAt) > 2*time.Second || c.State != "running" {
			interval = nil
		}
		if interval != nil {
			reason = ""
		}
		pairStart, pairEnd := now, now
		if c.Transport.PCRPairStart != nil {
			pairStart = *c.Transport.PCRPairStart
		}
		if c.Transport.PCRObservedAt != nil {
			pairEnd = *c.Transport.PCRObservedAt
		}
		add("stream_pcr_interval_seconds", "seconds", pairStart, pairEnd, interval, reason)
		add("stream_pcr_accuracy_seconds", "seconds", now, now, nil, "no calibrated reference clock; pipe timestamps are not a PCR accuracy measurement")
		if c.UDP != nil {
			add("stream_udp_submitted_bytes_total", "bytes", c.Transport.WindowStart, now, number(float64(c.UDP.Bytes)), "")
			add("stream_udp_write_errors_total", "errors", c.Transport.WindowStart, now, number(float64(c.UDP.Errors)), "")
			ready := 0.0
			if c.UDP.State == "ready" || c.UDP.State == "submitted_to_socket" {
				ready = 1
			}
			add("stream_udp_socket_available", "boolean", now, now, number(ready), "")
		}
		old, exists := s.previousStreams[c.ChannelID]
		if !exists || !old.StartedAt.Equal(c.StartedAt) {
			old = stream.Status{}
		}
		ccDelta := delta(c.Transport.CCErrors, old.Transport.CCErrors)
		udpDelta := uint64(0)
		if c.UDP != nil {
			previous := uint64(0)
			if old.UDP != nil && old.Attempts == c.Attempts {
				previous = old.UDP.Errors
			}
			udpDelta = delta(c.UDP.Errors, previous)
		}
		if old.Attempts != c.Attempts {
			ccDelta = c.Transport.CCErrors
		}
		conditions = append(conditions,
			condition{"stream_restarting", c.ChannelID, "warning", c.State == "backoff" || c.State == "starting" && c.Restarts > 0, 0},
			condition{"authorization_lost", c.ChannelID, "critical", c.State == "unauthorized", 0},
			condition{"channel_failed", c.ChannelID, "critical", c.State == "failed", 0},
			condition{"stream_stalled", c.ChannelID, "critical", c.State == "running" && !available, 5 * time.Second},
			condition{"continuity_errors", c.ChannelID, "warning", ccDelta > 0, 0},
			condition{"transport_errors", c.ChannelID, "warning", delta(c.TransportErrors, old.TransportErrors) > 0, 0},
			condition{"sync_errors", c.ChannelID, "critical", delta(c.SyncErrors, old.SyncErrors) > 0, 0})
		conditions = append(conditions, condition{"udp_write_errors", c.ChannelID, "warning", udpDelta > 0, 0})
		previous[c.ChannelID] = c
	}
	s.previousStreams = previous
	s.sampleNetwork(&result, network, now)
	s.evaluate(conditions, now)
	for _, alert := range s.active {
		result.Alerts = append(result.Alerts, alert)
	}
	sort.Slice(result.Alerts, func(i, j int) bool { return result.Alerts[i].ID < result.Alerts[j].ID })
	result.RecentAlerts = append(result.RecentAlerts, s.recent...)
	for i := range result.Measurements {
		m := &result.Measurements[i]
		if m.Timestamp.IsZero() {
			m.Timestamp = now
		}
		if m.WindowStart.IsZero() {
			m.WindowStart = m.Timestamp
		}
		if m.WindowEnd.IsZero() {
			m.WindowEnd = m.Timestamp
		}
		if m.Value != nil {
			m.Reason = ""
		}
	}
	sort.Slice(result.Channels, func(i, j int) bool { return result.Channels[i].ID < result.Channels[j].ID })
	s.current = result
}
func (s *Service) sampleNetwork(out *Snapshot, m diagnostics.Metric, now time.Time) {
	all, ok := m.Value.([]diagnostics.Interface)
	if m.State != "real" || !ok || m.Timestamp.IsZero() || now.Sub(m.Timestamp) > 3*time.Second || m.Timestamp.After(now) {
		s.previousNIC = map[string]networkSample{}
		for _, name := range []string{"network_rx_bits_per_second", "network_tx_bits_per_second"} {
			out.Measurements = append(out.Measurements, metric(name, "", "", m.Source, "host", "bits_per_second", m.Timestamp, m.Timestamp, nil, "network counters unavailable"))
		}
		return
	}
	if len(all) > 128 {
		all = all[:128]
	}
	next := map[string]networkSample{}
	for _, nic := range all {
		old, exists := s.previousNIC[nic.Name]
		seconds := m.Timestamp.Sub(old.at).Seconds()
		var rx, tx *float64
		reason := "requires two fresh monotonic counter samples"
		if exists && seconds > 0 && seconds <= 5 && nic.RXBytes >= old.rx && nic.TXBytes >= old.tx {
			rx = number(float64(nic.RXBytes-old.rx) * 8 / seconds)
			tx = number(float64(nic.TXBytes-old.tx) * 8 / seconds)
			reason = ""
		}
		start := old.at
		if !exists {
			start = m.Timestamp
		}
		out.Measurements = append(out.Measurements, metric("network_rx_bits_per_second", "", nic.Name, m.Source, "host", "bits_per_second", start, m.Timestamp, rx, reason), metric("network_tx_bits_per_second", "", nic.Name, m.Source, "host", "bits_per_second", start, m.Timestamp, tx, reason))
		next[nic.Name] = networkSample{m.Timestamp, nic.RXBytes, nic.TXBytes}
	}
	s.previousNIC = next
}
