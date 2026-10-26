package diagnostics

import (
	"bufio"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const maxRead = 1024 * 1024

var invalid = errors.New("invalid source")

type CPU struct {
	TotalTicks uint64 `json:"total_ticks"`
	IdleTicks  uint64 `json:"idle_ticks"`
}
type Memory struct {
	TotalBytes     uint64 `json:"total_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
}
type Disk struct {
	Path           string `json:"path"`
	TotalBytes     uint64 `json:"total_bytes"`
	FreeBytes      uint64 `json:"free_bytes"`
	AvailableBytes uint64 `json:"available_bytes"`
	UsedBytes      uint64 `json:"used_bytes"`
}
type Interface struct {
	Name    string `json:"name"`
	RXBytes uint64 `json:"rx_bytes"`
	TXBytes uint64 `json:"tx_bytes"`
}
type Route struct {
	Family      string `json:"family"`
	Interface   string `json:"interface"`
	Destination string `json:"destination"`
	Gateway     string `json:"gateway"`
	Flags       uint64 `json:"flags"`
}
type Sensor struct {
	Name    string  `json:"name"`
	Celsius float64 `json:"celsius"`
}

// readBounded never writes to procfs/sysfs. Limits also cover malformed fixtures.
func readBounded(root fs.FS, path string) (string, error) {
	f, err := root.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxRead+1))
	if err != nil {
		return "", err
	}
	if len(b) > maxRead {
		return "", invalid
	}
	return string(b), nil
}

func parseCPU(s string) (CPU, error) {
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 || f[0] != "cpu" {
			continue
		}
		if len(f) < 5 {
			return CPU{}, invalid
		}
		var c CPU
		// Guest times are already included in user/nice; do not double-count them.
		for i := 1; i < len(f) && i <= 8; i++ {
			n, e := strconv.ParseUint(f[i], 10, 64)
			if e != nil {
				return CPU{}, invalid
			}
			if ^uint64(0)-c.TotalTicks < n {
				return CPU{}, invalid
			}
			c.TotalTicks += n
			if i == 4 || i == 5 {
				c.IdleTicks += n
			}
		}
		if c.TotalTicks == 0 {
			return CPU{}, invalid
		}
		return c, nil
	}
	return CPU{}, invalid
}

func parseMemory(s string) (Memory, error) {
	values := map[string]uint64{}
	for _, line := range strings.Split(s, "\n") {
		f := strings.Fields(line)
		if len(f) == 0 {
			continue
		}
		if f[0] != "MemTotal:" && f[0] != "MemAvailable:" {
			continue
		}
		if len(f) != 3 || f[2] != "kB" {
			return Memory{}, invalid
		}
		n, e := strconv.ParseUint(f[1], 10, 64)
		if e != nil || n > ^uint64(0)/1024 {
			return Memory{}, invalid
		}
		values[f[0]] = n * 1024
	}
	total, ok := values["MemTotal:"]
	available, ok2 := values["MemAvailable:"]
	if !ok || !ok2 || total == 0 || available > total {
		return Memory{}, invalid
	}
	return Memory{total, available, total - available}, nil
}

func parseInterfaces(s string) ([]Interface, error) {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	if len(lines) < 2 {
		return nil, invalid
	}
	result := []Interface{}
	for _, line := range lines[2:] {
		name, data, ok := strings.Cut(line, ":")
		if !ok {
			return nil, invalid
		}
		f := strings.Fields(data)
		if len(f) != 16 {
			return nil, invalid
		}
		rx, e := strconv.ParseUint(f[0], 10, 64)
		tx, e2 := strconv.ParseUint(f[8], 10, 64)
		if e != nil || e2 != nil {
			return nil, invalid
		}
		result = append(result, Interface{strings.TrimSpace(name), rx, tx})
	}
	return result, nil
}

func ipv4(s string) (string, error) {
	n, e := strconv.ParseUint(s, 16, 32)
	if e != nil || len(s) != 8 {
		return "", invalid
	}
	b := make([]byte, 4)
	binary.NativeEndian.PutUint32(b, uint32(n))
	return net.IP(b).String(), nil
}
func ipv6(s string) (string, error) {
	b, e := hex.DecodeString(s)
	if e != nil || len(b) != 16 {
		return "", invalid
	}
	return net.IP(b).String(), nil
}

func parseRoutes(s string, v6 bool) ([]Route, error) {
	result := []Route{}
	scanner := bufio.NewScanner(strings.NewReader(s))
	first := true
	for scanner.Scan() {
		f := strings.Fields(scanner.Text())
		if len(f) == 0 {
			continue
		}
		if !v6 && first {
			first = false
			if f[0] != "Iface" {
				return nil, invalid
			}
			continue
		}
		var r Route
		var err, error2, flagsErr error
		if v6 {
			if len(f) != 10 {
				return nil, invalid
			}
			r.Family = "ipv6"
			r.Interface = f[9]
			r.Destination, err = ipv6(f[0])
			r.Gateway, error2 = ipv6(f[4])
			p, e := strconv.ParseUint(f[1], 16, 8)
			if e != nil || p > 128 {
				return nil, invalid
			}
			r.Destination += fmt.Sprintf("/%d", p)
			r.Flags, flagsErr = strconv.ParseUint(f[8], 16, 64)
		} else {
			if len(f) < 11 {
				return nil, invalid
			}
			r.Family = "ipv4"
			r.Interface = f[0]
			r.Destination, err = ipv4(f[1])
			r.Gateway, error2 = ipv4(f[2])
			mask, e := ipv4(f[7])
			if e != nil {
				return nil, invalid
			}
			bits, size := net.IPMask(net.ParseIP(mask).To4()).Size()
			if size != 32 {
				return nil, invalid
			}
			r.Destination += fmt.Sprintf("/%d", bits)
			r.Flags, flagsErr = strconv.ParseUint(f[3], 16, 64)
		}
		if err != nil || error2 != nil || flagsErr != nil {
			return nil, invalid
		}
		result = append(result, r)
	}
	if scanner.Err() != nil {
		return nil, scanner.Err()
	}
	if !v6 && first {
		return nil, invalid
	}
	return result, nil
}

func sensors(root fs.FS) ([]Sensor, error) {
	entries, err := fs.ReadDir(root, "sys/class/thermal")
	if err != nil {
		return nil, err
	}
	result := []Sensor{}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "thermal_zone") {
			continue
		}
		if len(result) >= 128 {
			return nil, invalid
		}
		base := filepath.ToSlash(filepath.Join("sys/class/thermal", entry.Name()))
		raw, e := readBounded(root, base+"/temp")
		if e != nil {
			continue
		}
		n, e := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if e != nil || n < -273150 || n > 1000000 {
			continue
		}
		result = append(result, Sensor{entry.Name(), float64(n) / 1000})
	}
	if len(result) == 0 {
		return nil, invalid
	}
	return result, nil
}

func collect(root fs.FS, disk func() (Disk, error), now time.Time) Snapshot {
	s := Snapshot{Timestamp: now}
	raw, e := readBounded(root, "proc/stat")
	var cpu CPU
	if e == nil {
		cpu, e = parseCPU(raw)
	}
	s.CPU = observed(now, "/proc/stat", cpu, e)
	raw, e = readBounded(root, "proc/meminfo")
	var memory Memory
	if e == nil {
		memory, e = parseMemory(raw)
	}
	s.Memory = observed(now, "/proc/meminfo", memory, e)
	raw, e = readBounded(root, "proc/net/dev")
	var interfaces []Interface
	if e == nil {
		interfaces, e = parseInterfaces(raw)
	}
	s.Interfaces = observed(now, "/proc/net/dev", interfaces, e)
	raw, e = readBounded(root, "proc/net/route")
	var routes []Route
	if e == nil {
		routes, e = parseRoutes(raw, false)
	}
	raw6, e6 := readBounded(root, "proc/net/ipv6_route")
	var routes6 []Route
	if e6 == nil {
		routes6, e6 = parseRoutes(raw6, true)
	}
	// Do not silently present an incomplete routing table as complete.
	s.Routes = observed(now, "/proc/net/route + /proc/net/ipv6_route", append(routes, routes6...), errors.Join(e, e6))
	d, e := disk()
	s.Disk = observed(now, "statfs:/", d, e)
	temps, e := sensors(root)
	s.Sensors = observed(now, "/sys/class/thermal", temps, e)
	return s
}
