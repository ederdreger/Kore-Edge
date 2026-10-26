package monitoring

import (
	"kore-edge/internal/stream"
	"testing"
	"time"
)

func TestOperationalAlertsAreDeduplicatedAndResolve(t *testing.T) {
	now := time.Unix(1000, 0).UTC()
	s := New(nil, nil)
	bad := channel(now)
	bad.ChannelID = "retrying"
	bad.State = "backoff"
	good := channel(now)
	good.ChannelID = "healthy"
	for i := 0; i < 20; i++ {
		at := now.Add(time.Duration(i) * time.Second)
		good.LastPacketAt = &at
		s.Sample(at, []stream.Status{bad, good}, network(at, 0, 0))
		snap := s.Snapshot(at)
		if len(snap.Alerts) != 1 || !hasAlert(snap, "stream_restarting", "firing") {
			t.Fatal("alert duplicated or healthy channel affected")
		}
	}
	at := now.Add(20 * time.Second)
	bad.State = "failed"
	s.Sample(at, []stream.Status{bad}, network(at, 0, 0))
	snap := s.Snapshot(at)
	if !hasAlert(snap, "channel_failed", "firing") || len(snap.RecentAlerts) != 1 || snap.RecentAlerts[0].Code != "stream_restarting" {
		t.Fatal("retry exhaustion alert transition")
	}
	at = at.Add(time.Second)
	bad.State = "unauthorized"
	s.Sample(at, []stream.Status{bad}, network(at, 0, 0))
	if !hasAlert(s.Snapshot(at), "authorization_lost", "firing") {
		t.Fatal("missing authorization alert")
	}
}
