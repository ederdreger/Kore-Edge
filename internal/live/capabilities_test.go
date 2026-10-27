package live

import (
	"context"
	"errors"
	"kore-edge/internal/multicast"
	"path/filepath"
	"runtime"
	"testing"
)

func TestUnavailableInterfaceInventoryDoesNotDisableChannelConfiguration(t *testing.T) {
	catalog := &syntheticCatalog{}
	catalog.allowed.Store(true)
	m, e := New(context.Background(), filepath.Join(t.TempDir(), "live"), catalog)
	if e != nil {
		t.Fatal(e)
	}
	defer m.Close()
	m.interfaces = func() ([]multicast.Interface, error) { return nil, errors.New("read-only inventory unavailable") }
	cap, e := m.Capabilities()
	if e != nil || cap.InterfacesAvailable || cap.Interfaces == nil || len(cap.Interfaces) != 0 || cap.Warning == "" || cap.Platform != runtime.GOOS {
		t.Fatalf("partial capability discovery failed: %+v %v", cap, e)
	}
	if e = m.Save(ChannelConfig{ChannelID: "synthetic-one", Protocol: "http_ts"}, cap.Tools, 0); e != nil {
		t.Fatal("HTTP configuration depended on interface inventory", e)
	}
	if _, e = m.PlanUDP(multicast.Target{ChannelID: "synthetic-one"}); e == nil {
		t.Fatal("UDP target accepted without authorization/inventory")
	}
}
