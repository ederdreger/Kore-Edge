//go:build linux

package multicast

import (
	"context"
	"errors"
	"net"
	"syscall"
	"time"
)

type linuxOptions struct{ fd int }

func (o linuxOptions) Device(name string) error {
	return syscall.SetsockoptString(o.fd, syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, name)
}
func (o linuxOptions) Interface(index int, ip string) error {
	var addr [4]byte
	copy(addr[:], net.ParseIP(ip).To4())
	return syscall.SetsockoptIPMreqn(o.fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_IF, &syscall.IPMreqn{Ifindex: int32(index), Address: addr})
}
func (o linuxOptions) TTL(ttl int) error {
	return syscall.SetsockoptInt(o.fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_TTL, ttl)
}
func (o linuxOptions) Loopback(on bool) error {
	n := 0
	if on {
		n = 1
	}
	return syscall.SetsockoptInt(o.fd, syscall.IPPROTO_IP, syscall.IP_MULTICAST_LOOP, n)
}
func (o linuxOptions) Buffer(n int) error {
	return syscall.SetsockoptInt(o.fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, n)
}

func openSocket(t Target, index int) (Socket, error) {
	dialer := net.Dialer{Timeout: time.Second, LocalAddr: &net.UDPAddr{IP: net.ParseIP(t.SourceIP)}, Control: func(network, address string, raw syscall.RawConn) error {
		var optionErr error
		e := raw.Control(func(fd uintptr) { optionErr = applyOptions(t, index, linuxOptions{int(fd)}) })
		if e != nil {
			return e
		}
		return optionErr
	}}
	conn, e := dialer.DialContext(context.Background(), "udp4", net.JoinHostPort(t.Group, fmtPort(t.Port)))
	if e != nil {
		return nil, errors.New("multicast socket setup failed")
	}
	return conn, nil
}
