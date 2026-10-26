package diagnostics

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func fixture() fstest.MapFS {
	return fstest.MapFS{
		"proc/stat":                            {Data: []byte("cpu 10 2 3 40 5 1 2 0 8 9\n")},
		"proc/meminfo":                         {Data: []byte("MemTotal: 100 kB\nMemAvailable: 40 kB\n")},
		"proc/net/dev":                         {Data: []byte("Inter-| Receive | Transmit\n face |bytes\n lo: 100 0 0 0 0 0 0 0 200 0 0 0 0 0 0 0\n")},
		"proc/net/route":                       {Data: []byte("Iface Destination Gateway Flags RefCnt Use Metric Mask MTU Window IRTT\nlab0 00000000 0100007F 0003 0 0 0 00000000 0 0 0\n")},
		"proc/net/ipv6_route":                  {Data: []byte(strings.Repeat("0", 32) + " 00 " + strings.Repeat("0", 32) + " 00 " + strings.Repeat("0", 31) + "1 00000000 00000000 00000000 00000001 lo\n")},
		"sys/class/thermal/thermal_zone0/temp": {Data: []byte("42000\n")},
	}
}

func TestSyntheticSnapshot(t *testing.T) {
	now := time.Date(2026, 10, 25, 12, 0, 0, 0, time.UTC)
	s := collect(fixture(), func() (Disk, error) { return Disk{"/", 100, 40, 30, 60}, nil }, now)
	for _, m := range []Metric{s.CPU, s.Memory, s.Disk, s.Interfaces, s.Routes, s.Sensors} {
		if m.State != "real" || !m.Timestamp.Equal(now) || m.Source == "" || m.Value == nil {
			t.Fatalf("unexpected metric: %+v", m)
		}
	}
	if s.CPU.Value.(CPU).TotalTicks != 63 || s.CPU.Value.(CPU).IdleTicks != 45 {
		t.Fatal("guest accounting")
	}
	if s.Memory.Value.(Memory).UsedBytes != 60*1024 {
		t.Fatal("memory")
	}
	if s.Interfaces.Value.([]Interface)[0].TXBytes != 200 {
		t.Fatal("network")
	}
	routes := s.Routes.Value.([]Route)
	if len(routes) != 2 || routes[0].Gateway != "127.0.0.1" || routes[1].Gateway != "::1" {
		t.Fatalf("routes: %+v", routes)
	}
	if s.Sensors.Value.([]Sensor)[0].Celsius != 42 {
		t.Fatal("temperature")
	}
	if _, e := json.Marshal(s); e != nil {
		t.Fatal(e)
	}
}

func TestMissingSources(t *testing.T) {
	s := collect(fstest.MapFS{}, func() (Disk, error) { return Disk{}, invalid }, time.Now().UTC())
	for _, m := range []Metric{s.CPU, s.Memory, s.Disk, s.Interfaces, s.Routes, s.Sensors} {
		if m.State != "unavailable" || m.Value != nil || m.Reason == "" {
			t.Fatalf("unexpected: %+v", m)
		}
	}
}

func TestPartialFailure(t *testing.T) {
	f := fixture()
	delete(f, "proc/net/ipv6_route")
	f["proc/meminfo"] = &fstest.MapFile{Data: []byte("MemTotal: bad kB")}
	s := collect(f, func() (Disk, error) { return Disk{}, invalid }, time.Now())
	if s.Routes.State != "unavailable" || s.Memory.State != "unavailable" || s.CPU.State != "real" {
		t.Fatal("partial failure must be explicit")
	}
}

func TestMalformed(t *testing.T) {
	for _, input := range []string{"", "cpu x 0 0 0", "cpu 1", "cpu 0 0 0 0"} {
		if _, e := parseCPU(input); e == nil {
			t.Fatalf("accepted CPU %q", input)
		}
	}
	for _, input := range []string{"", "MemTotal: 1 kB\nMemAvailable: 2 kB", "MemTotal: 1 MB\nMemAvailable: 0 kB"} {
		if _, e := parseMemory(input); e == nil {
			t.Fatal("accepted memory")
		}
	}
	if _, e := parseInterfaces("header\nheader\nlo: x"); e == nil {
		t.Fatal("accepted interface")
	}
	if _, e := parseRoutes("Iface\nbad", false); e == nil {
		t.Fatal("accepted route")
	}
	if _, e := parseRoutes("bad", true); e == nil {
		t.Fatal("accepted ipv6")
	}
	if _, e := readBounded(fstest.MapFS{"large": {Data: []byte(strings.Repeat("x", maxRead+1))}}, "large"); e == nil {
		t.Fatal("unbounded read")
	}
}
