package monitoring

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"kore-edge/internal/diagnostics"
	"kore-edge/internal/multicast"
	"kore-edge/internal/stream"
	"kore-edge/internal/tsmetrics"
)

func network(at time.Time, rx, tx uint64) diagnostics.Metric {
	return diagnostics.Metric{Timestamp: at, State: "real", Source: "/proc/net/dev", Value: []diagnostics.Interface{{Name: "lab0", RXBytes: rx, TXBytes: tx}}}
}
func find(t *testing.T, s Snapshot, name string) Measurement {
	t.Helper()
	for _, m := range s.Measurements {
		if m.Name == name {
			return m
		}
	}
	t.Fatalf("missing %s", name)
	return Measurement{}
}
func channel(now time.Time) stream.Status {
	return stream.Status{ChannelID: "test", State: "running", Source: "synthetic:null-ts", StartedAt: now.Add(-time.Minute), LastPacketAt: &now, Attempts: 1, Bytes: 1880, Packets: 10, Bitrate: stream.Rate{State: "measured_synthetic", BitsPerSecond: number(15040), WindowSeconds: 1, WindowStart: now.Add(-time.Second), WindowEnd: now}, Transport: tsmetrics.Snapshot{WindowStart: now.Add(-time.Minute), WindowEnd: now, CCState: "unavailable", PCRState: "unavailable"}}
}
func TestNetworkWindowsResetsAndUnavailable(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s := New(nil, nil)
	s.Sample(now, nil, network(now, 1000, 2000))
	if find(t, s.Snapshot(now), "network_rx_bits_per_second").Value != nil {
		t.Fatal("first sample invented rate")
	}
	s.Sample(now.Add(2*time.Second), nil, network(now.Add(2*time.Second), 2000, 4000))
	m := find(t, s.Snapshot(now.Add(2*time.Second)), "network_rx_bits_per_second")
	if m.Value == nil || *m.Value != 4000 || m.WindowEnd.Sub(m.WindowStart) != 2*time.Second || m.Origin != "host" {
		t.Fatalf("rate %+v", m)
	}
	s.Sample(now.Add(3*time.Second), nil, network(now.Add(3*time.Second), 1, 1))
	if find(t, s.Snapshot(now.Add(3*time.Second)), "network_rx_bits_per_second").Value != nil {
		t.Fatal("counter reset")
	}
	s.Sample(now.Add(4*time.Second), nil, diagnostics.Metric{State: "unavailable", Source: "linux"})
	if find(t, s.Snapshot(now.Add(4*time.Second)), "network_rx_bits_per_second").State != "unavailable" {
		t.Fatal("unsupported host")
	}
}
func TestProvenanceAndStaleness(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s := New(func() []stream.Status { return nil }, nil)
	c := channel(now)
	s.Sample(now, []stream.Status{c}, network(now, 1, 2))
	snapshot := s.Snapshot(now)
	m := find(t, snapshot, "stream_ingest_bitrate_bits_per_second")
	if m.Value == nil || *m.Value != 15040 || m.Origin != "synthetic" || m.WindowEnd.Sub(m.WindowStart) != time.Second {
		t.Fatal("bitrate provenance")
	}
	if find(t, snapshot, "stream_cc_errors_total").Value != nil || find(t, snapshot, "stream_pcr_interval_seconds").Value != nil || find(t, snapshot, "stream_pcr_accuracy_seconds").Value != nil {
		t.Fatal("unavailable TS metrics invented")
	}
	*m.Value = 999
	if *find(t, s.Snapshot(now), "stream_ingest_bitrate_bits_per_second").Value == 999 {
		t.Fatal("mutable snapshot")
	}
	stale := s.Snapshot(now.Add(4 * time.Second))
	if stale.State != "stale" || find(t, stale, "stream_ingest_bitrate_bits_per_second").Value != nil {
		t.Fatal("stale values published")
	}
}
func hasAlert(s Snapshot, code, state string) bool {
	for _, a := range s.Alerts {
		if a.Code == code && a.State == state {
			return true
		}
	}
	return false
}
func TestAlertPendingFiringResolved(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s := New(nil, nil)
	c := channel(now)
	last := now.Add(-3 * time.Second)
	c.LastPacketAt = &last
	s.Sample(now, []stream.Status{c}, network(now, 0, 0))
	if !hasAlert(s.Snapshot(now), "stream_stalled", "pending") {
		t.Fatal("missing pending")
	}
	for i := 1; i <= 5; i++ {
		stamp := now.Add(time.Duration(i) * time.Second)
		s.Sample(stamp, []stream.Status{c}, network(stamp, 0, 0))
	}
	if !hasAlert(s.Snapshot(now.Add(5*time.Second)), "stream_stalled", "firing") {
		t.Fatal("missing firing")
	}
	c.State = "stopped"
	s.Sample(now.Add(6*time.Second), []stream.Status{c}, network(now.Add(6*time.Second), 0, 0))
	got := s.Snapshot(now.Add(6 * time.Second))
	if len(got.Alerts) != 0 || len(got.RecentAlerts) != 1 || got.RecentAlerts[0].State != "resolved" {
		t.Fatal("resolution")
	}
	c.State = "failed"
	s.Sample(now.Add(7*time.Second), []stream.Status{c}, network(now.Add(7*time.Second), 0, 0))
	if !hasAlert(s.Snapshot(now.Add(7*time.Second)), "channel_failed", "firing") {
		t.Fatal("failure alert")
	}
}
func TestErrorAlertsAndBoundedHistory(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s := New(nil, nil)
	c := channel(now)
	c.Transport.CCErrors = 2
	c.Transport.CCState = "measured"
	c.TransportErrors = 1
	c.SyncErrors = 1
	c.UDP = &multicast.Stats{State: "failed", Errors: 1}
	s.Sample(now, []stream.Status{c}, network(now, 0, 0))
	got := s.Snapshot(now)
	for _, code := range []string{"continuity_errors", "transport_errors", "sync_errors", "udp_write_errors"} {
		if !hasAlert(got, code, "firing") {
			t.Fatal("missing TS alert", code)
		}
	}
	s.Sample(now.Add(time.Second), []stream.Status{c}, network(now.Add(time.Second), 0, 0))
	if len(s.Snapshot(now.Add(time.Second)).Alerts) != 0 {
		t.Fatal("counter treated as perpetual delta")
	}
	for i := 0; i < 150; i++ {
		stamp := now.Add(time.Duration(i+2) * time.Second)
		c.State = "failed"
		s.Sample(stamp, []stream.Status{c}, network(stamp, 0, 0))
		c.State = "stopped"
		s.Sample(stamp, []stream.Status{c}, network(stamp, 0, 0))
	}
	if len(s.Snapshot(now).RecentAlerts) != 128 {
		t.Fatal("unbounded history")
	}
}
func TestExporterEscapesAndOmitsUnavailable(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s := Snapshot{Measurements: []Measurement{metric("stream_ingest_bitrate_bits_per_second", "x\"\n\\", "", "pipe", "synthetic", "bits_per_second", now, now.Add(time.Second), number(1000), ""), metric("stream_pcr_accuracy_seconds", "test", "", "pipe", "synthetic", "seconds", now, now, nil, "unavailable")}}
	var out bytes.Buffer
	Export(&out, s)
	text := out.String()
	if !strings.Contains(text, `channel_id="x\"\n\\"`) || !strings.Contains(text, "# TYPE kore_edge_stream_ingest_bitrate_bits_per_second gauge") || strings.Contains(text, "kore_edge_stream_pcr_accuracy_seconds{") {
		t.Fatal("invalid export", text)
	}
	if !strings.HasSuffix(text, "\n") {
		t.Fatal("missing final LF")
	}
}
func TestSamplerCancellation(t *testing.T) {
	s := New(nil, func() diagnostics.Metric { return diagnostics.Metric{State: "unavailable"} })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan struct{})
	go func() { s.Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("sampler did not stop")
	}
}

func TestNetworkGapAndClockReversal(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s := New(nil, nil)
	s.Sample(now, nil, network(now, 1, 1))
	s.Sample(now.Add(6*time.Second), nil, network(now.Add(6*time.Second), 10, 10))
	if find(t, s.Snapshot(now.Add(6*time.Second)), "network_rx_bits_per_second").Value != nil {
		t.Fatal("rate across stale gap")
	}
	s.Sample(now.Add(5*time.Second), nil, network(now.Add(5*time.Second), 20, 20))
	if find(t, s.Snapshot(now.Add(5*time.Second)), "network_rx_bits_per_second").Value != nil {
		t.Fatal("negative clock window")
	}
}
func TestPendingAlertResetsAcrossSamplerGap(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s := New(nil, nil)
	c := channel(now)
	last := now.Add(-3 * time.Second)
	c.LastPacketAt = &last
	s.Sample(now, []stream.Status{c}, network(now, 0, 0))
	later := now.Add(10 * time.Second)
	s.Sample(later, []stream.Status{c}, network(later, 0, 0))
	if !hasAlert(s.Snapshot(later), "stream_stalled", "pending") {
		t.Fatal("claimed continuous observation across a gap")
	}
	c.State = "stopped"
	s.Sample(later.Add(time.Second), []stream.Status{c}, network(later.Add(time.Second), 0, 0))
	if len(s.Snapshot(later.Add(time.Second)).RecentAlerts) != 0 {
		t.Fatal("resolved a never-fired alert")
	}
}
