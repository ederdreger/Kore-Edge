//go:build linux

package multicast

import (
	"errors"
	"golang.org/x/sys/unix"
	"net/netip"
	"os"
)

// Go's net.Interfaces uses AF_NETLINK, which is deliberately unavailable in
// the installed service. These GET ioctls use an unbound AF_INET socket: no
// address, flags, routes or socket destinations are changed and no packet is sent.
func linuxCompatibleInterfaces() ([]Interface, error) {
	entries, e := os.ReadDir("/sys/class/net")
	if e != nil {
		return nil, errors.New("interface inventory unavailable")
	}
	fd, e := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM|unix.SOCK_CLOEXEC, 0)
	if e != nil {
		return nil, errors.New("interface inventory unavailable")
	}
	defer unix.Close(fd)
	result := []Interface{}
	for _, entry := range entries {
		name := entry.Name()
		flags, e := unix.NewIfreq(name)
		if e != nil {
			continue
		}
		if e = unix.IoctlIfreq(fd, unix.SIOCGIFFLAGS, flags); e != nil {
			if errors.Is(e, unix.ENODEV) {
				continue
			}
			return nil, errors.New("interface inventory unavailable")
		}
		index, _ := unix.NewIfreq(name)
		mtu, _ := unix.NewIfreq(name)
		if unix.IoctlIfreq(fd, unix.SIOCGIFINDEX, index) != nil || unix.IoctlIfreq(fd, unix.SIOCGIFMTU, mtu) != nil {
			continue
		}
		f := flags.Uint16()
		nic := Interface{Name: name, Index: int(index.Uint32()), MTU: int(mtu.Uint32()), Up: f&unix.IFF_UP != 0, Loopback: f&unix.IFF_LOOPBACK != 0, PointToPoint: f&unix.IFF_POINTOPOINT != 0, Multicast: f&unix.IFF_MULTICAST != 0, IPv4: []string{}, IPv4PrimaryOnly: true}
		addr, _ := unix.NewIfreq(name)
		e = unix.IoctlIfreq(fd, unix.SIOCGIFADDR, addr)
		if e == nil {
			bytes, e := addr.Inet4Addr()
			if e == nil && len(bytes) == 4 {
				nic.IPv4 = append(nic.IPv4, netip.AddrFrom4([4]byte{bytes[0], bytes[1], bytes[2], bytes[3]}).String())
			}
		} else if !errors.Is(e, unix.EADDRNOTAVAIL) && !errors.Is(e, unix.ENODEV) {
			return nil, errors.New("interface address inventory unavailable")
		}
		result = append(result, nic)
	}
	if len(result) == 0 {
		return nil, errors.New("interface inventory unavailable")
	}
	return result, nil
}
