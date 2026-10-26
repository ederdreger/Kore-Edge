package config

import (
	"strings"
	"testing"

	"kore-edge/internal/multicast"
)

func TestMulticastConfigurationAndSnapshotIsolation(t *testing.T) {
	c, e := Parse(strings.NewReader(valid))
	if e != nil {
		t.Fatal(e)
	}
	c.Stream.Enabled = true
	c.Catalog.PartnerID = "partner-lab"
	c.Catalog.PublicKey = "AQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQEBAQE="
	c.Multicast = multicast.Config{Mode: "dry_run", Outputs: []multicast.Target{{ChannelID: "test", Interface: "lab0", SourceIP: "10.250.0.2", Group: "239.18.10.1", Port: 12000, TTL: 1}}}
	var store Store
	if store.Apply(c) != nil {
		t.Fatal("valid config")
	}
	c.Multicast.Outputs[0].TTL = 2
	got, _ := store.Snapshot()
	if got.Multicast.Outputs[0].TTL != 1 {
		t.Fatal("mutable input")
	}
	got.Multicast.Outputs[0].TTL = 3
	again, _ := store.Snapshot()
	if again.Multicast.Outputs[0].TTL != 1 {
		t.Fatal("mutable snapshot")
	}
	if store.Apply(c) == nil {
		t.Fatal("unsafe TTL accepted")
	}
	got, _ = store.Snapshot()
	if got.Multicast.Outputs[0].TTL != 1 {
		t.Fatal("invalid replacement")
	}
	c = again
	c.Stream.Enabled = false
	if c.Validate() == nil {
		t.Fatal("output without ingest")
	}
}
