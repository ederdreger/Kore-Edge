//go:build !linux

package diagnostics

import "testing"

func TestUnsupportedHost(t *testing.T) {
	s := Collect()
	if s.CPU.State != "unavailable" || s.CPU.Value != nil || s.Timestamp.IsZero() {
		t.Fatal("misrepresented unsupported host")
	}
}
