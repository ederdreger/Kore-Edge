//go:build linux

package diagnostics

import (
	"os"
	"syscall"
	"time"
)

func Collect() Snapshot {
	return collect(os.DirFS("/"), func() (Disk, error) {
		var stat syscall.Statfs_t
		if err := syscall.Statfs("/", &stat); err != nil {
			return Disk{}, err
		}
		if stat.Bsize <= 0 {
			return Disk{}, invalid
		}
		size := uint64(stat.Bsize)
		if stat.Blocks > ^uint64(0)/size || stat.Bfree > stat.Blocks || stat.Bavail > stat.Blocks {
			return Disk{}, invalid
		}
		return Disk{"/", stat.Blocks * size, stat.Bfree * size, stat.Bavail * size, (stat.Blocks - stat.Bfree) * size}, nil
	}, time.Now().UTC())
}

func Network() Metric {
	now := time.Now().UTC()
	raw, e := readBounded(os.DirFS("/"), "proc/net/dev")
	var nic []Interface
	if e == nil {
		nic, e = parseInterfaces(raw)
	}
	return observed(now, "/proc/net/dev", nic, e)
}
