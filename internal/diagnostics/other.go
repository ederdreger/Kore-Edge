//go:build !linux

package diagnostics

import "time"

func Collect() Snapshot {
	now := time.Now().UTC()
	m := unavailable(now, "linux", "unsupported operating system")
	return Snapshot{Timestamp: now, CPU: m, Memory: m, Disk: m, Interfaces: m, Routes: m, Sensors: m}
}

func Network() Metric {
	return unavailable(time.Now().UTC(), "/proc/net/dev", "unsupported operating system")
}
