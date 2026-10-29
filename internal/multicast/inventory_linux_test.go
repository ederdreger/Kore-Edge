//go:build linux

package multicast

import (
	"golang.org/x/sys/unix"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"unsafe"
)

func TestLinuxReadOnlyInterfaceInventory(t *testing.T) {
	all, e := linuxCompatibleInterfaces()
	if e != nil {
		t.Fatal(e)
	}
	found := false
	for _, nic := range all {
		if nic.Index < 1 || nic.MTU < 68 || !nic.IPv4PrimaryOnly {
			t.Fatal("invalid inventory")
		}
		if nic.Loopback {
			found = true
			if len(nic.IPv4) == 0 || nic.IPv4[0] != "127.0.0.1" {
				t.Fatal("loopback address unavailable")
			}
		}
	}
	if !found {
		t.Fatal("no loopback inventory")
	}
}

// Reproduce the service's blocked address family inside a disposable child.
// This never modifies systemd, NICs, routes, firewall or the parent process.
func TestRestrictedLinuxInventory(t *testing.T) {
	if runtime.GOARCH != "amd64" && runtime.GOARCH != "arm64" {
		t.Skip("seccomp test requires a supported little-endian architecture")
	}
	if os.Getenv("KORE_TEST_DENY_NETLINK") != "1" {
		cmd := exec.Command(os.Args[0], "-test.run=^TestRestrictedLinuxInventory$", "-test.v")
		cmd.Env = append(os.Environ(), "KORE_TEST_DENY_NETLINK=1")
		out, e := cmd.CombinedOutput()
		if strings.Contains(string(out), "seccomp unavailable") {
			t.Skip("seccomp unavailable")
		}
		if e != nil || !strings.Contains(string(out), "restricted inventory verified") {
			t.Fatal("restricted inventory failed", string(out), e)
		}
		return
	}
	filters := []unix.SockFilter{{Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 0}, {Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.SYS_SOCKET, Jf: 3}, {Code: unix.BPF_LD | unix.BPF_W | unix.BPF_ABS, K: 16}, {Code: unix.BPF_JMP | unix.BPF_JEQ | unix.BPF_K, K: unix.AF_NETLINK, Jf: 1}, {Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ERRNO | uint32(unix.EPERM)}, {Code: unix.BPF_RET | unix.BPF_K, K: unix.SECCOMP_RET_ALLOW}}
	if unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0) != nil {
		t.Skip("seccomp unavailable")
	}
	prog := unix.SockFprog{Len: uint16(len(filters)), Filter: &filters[0]}
	_, _, errno := unix.Syscall6(unix.SYS_SECCOMP, unix.SECCOMP_SET_MODE_FILTER, unix.SECCOMP_FILTER_FLAG_TSYNC, uintptr(unsafe.Pointer(&prog)), 0, 0, 0)
	runtime.KeepAlive(filters)
	if errno != 0 {
		t.Skip("seccomp unavailable")
	}
	if _, e := net.Interfaces(); e == nil {
		t.Fatal("NETLINK was not denied")
	}
	all, e := Interfaces()
	if e != nil || len(all) == 0 || !all[0].IPv4PrimaryOnly {
		t.Fatal("inventory unavailable with NETLINK denied", e)
	}
	t.Log("restricted inventory verified")
}
