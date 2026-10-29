package multicast

import (
	"net"
	"syscall"
	"testing"
)

func TestInventoryFallsBackWhenKernelInterfaceEnumerationIsDenied(t *testing.T) {
	called := false
	got, e := interfaceInventory(func() ([]net.Interface, error) { return nil, syscall.EPERM }, func() ([]Interface, error) {
		called = true
		return []Interface{{Name: "synthetic0", Up: true, Multicast: true, IPv4: []string{"192.0.2.10"}}}, nil
	})
	if e != nil || !called || len(got) != 1 || got[0].IPv4[0] != "192.0.2.10" {
		t.Fatal("restricted inventory fallback failed")
	}
}
