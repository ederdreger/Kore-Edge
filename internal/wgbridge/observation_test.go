package wgbridge

import (
	"context"
	"fmt"
	"kore-edge/internal/wireguard"
	"testing"
)

func TestPeerObservationExpiresAndDoesNotCountOtherKeys(t *testing.T) {
	peers := []wireguard.Peer{{PublicKey: "edge-a"}, {PublicKey: "edge-b"}}
	rows, ok := observeHandshakes(peers, "edge-a\t990\nedge-b\t800\n", 1000)
	if !ok || !rows[0].Online || rows[1].Online || rows[0].LastHandshake != 990 {
		t.Fatal("per-peer freshness incorrect")
	}
	rows, ok = observeHandshakes(peers, "edge-a\t990\nedge-b\t0\n", 1200)
	if !ok || rows[0].Online || rows[1].Online {
		t.Fatal("stale reply or never connected peer marked online")
	}
	for _, raw := range []string{"edge-a 990\nstranger 990", "edge-a 990\nedge-a 990", "edge-a broken\nedge-b 990", "", "edge-a 990"} {
		if _, ok := observeHandshakes(peers, raw, 1000); ok {
			t.Fatal("unknown observation accepted", raw)
		}
	}
}

func TestStatusReturnsTimestampAndPerPeerEvidence(t *testing.T) {
	e, k, c := fake(t)
	p, err := e.Plan(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Apply(p.Digest); err != nil {
		t.Fatal(err)
	}
	wait(t, e)
	k.mu.Lock()
	k.handshake = true
	k.mu.Unlock()
	status := e.Status(context.Background())
	if !status.ObservationAvailable || status.CheckedAt == "" || len(status.Peers) != 1 || status.Peers[0].PublicKey != c.Peers[0].PublicKey || !status.Peers[0].Online {
		t.Fatal("missing evidence")
	}
	k.mu.Lock()
	k.fail = "latest-handshakes"
	k.mu.Unlock()
	status = e.Status(context.Background())
	if status.HandshakeRecent || status.ObservationAvailable || status.Stage != "observation_unavailable" {
		t.Fatal(fmt.Sprintf("unavailable was shown as confirmed: %+v", status))
	}
}
