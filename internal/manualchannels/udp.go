package manualchannels

import (
	"context"
	"io"
	"net"
	"strconv"
	"time"
)

func (s *Service) udpTarget(c Source) (*net.UDPAddr, error) {
	u, e := parse(c.URL)
	if e != nil {
		return nil, e
	}
	group := net.ParseIP(u.Hostname())
	port, _ := strconv.Atoi(u.Port())
	all, e := s.interfaces()
	if e != nil {
		return nil, ErrUnavailable
	}
	for _, nic := range all {
		if nic.Name != c.Interface || !nic.Up {
			continue
		}
		if group.IsMulticast() && !nic.Multicast {
			return nil, ErrInvalid
		}
		for _, ip := range nic.IPv4 {
			if ip == c.LocalIP {
				if !group.IsMulticast() && u.Hostname() != c.LocalIP {
					return nil, ErrInvalid
				}
				return &net.UDPAddr{IP: group, Port: port}, nil
			}
		}
	}
	return nil, ErrInvalid
}

type datagrams struct {
	conn    *net.UDPConn
	pending []byte
	buffer  []byte
}

func (d *datagrams) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if len(d.pending) == 0 {
		_ = d.conn.SetReadDeadline(time.Now().Add(8 * time.Second))
		n, _, e := d.conn.ReadFromUDP(d.buffer)
		if e != nil {
			return 0, ErrUnavailable
		}
		if n == 0 || n%188 != 0 {
			return 0, ErrInvalid
		}
		for i := 0; i < n; i += 188 {
			if d.buffer[i] != 0x47 {
				return 0, ErrInvalid
			}
		}
		d.pending = d.buffer[:n]
	}
	n := copy(p, d.pending)
	d.pending = d.pending[n:]
	return n, nil
}
func (d *datagrams) Close() error { return d.conn.Close() }
func (s *Service) openUDP(ctx context.Context, c Source) (io.ReadCloser, error) {
	address, e := s.udpTarget(c)
	if e != nil {
		return nil, e
	}
	var conn *net.UDPConn
	if address.IP.IsMulticast() {
		nic, e := net.InterfaceByName(c.Interface)
		if e != nil {
			return nil, ErrUnavailable
		}
		conn, e = net.ListenMulticastUDP("udp4", nic, address)
		if e != nil {
			return nil, ErrUnavailable
		}
	} else {
		conn, e = net.ListenUDP("udp4", address)
		if e != nil {
			return nil, ErrUnavailable
		}
	}
	if conn.SetReadBuffer(2*1024*1024) != nil {
		conn.Close()
		return nil, ErrUnavailable
	}
	return contextual(ctx, &datagrams{conn: conn, buffer: make([]byte, 65535)}), nil
}
