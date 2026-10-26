//go:build linux

package diagnostics

import "testing"

func TestHostSnapshot(t *testing.T) {
	s := Collect()
	if s.Timestamp.IsZero() {
		t.Fatal("missing timestamp")
	}
	for _, m := range []Metric{s.CPU, s.Memory, s.Disk, s.Interfaces, s.Routes, s.Sensors} {
		if m.State != "real" && m.State != "unavailable" {
			t.Fatal("invalid state")
		}
	}
}
