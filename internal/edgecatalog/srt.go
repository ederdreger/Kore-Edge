package edgecatalog

import (
	"context"
	srt "github.com/datarhei/gosrt"
	"io"
	"net"
	"net/netip"
	"strconv"
	"sync"
	"time"
)

type SRTTransport struct {
	Port      int `json:"port"`
	LatencyMS int `json:"latency_ms"`
}
type srtInput struct {
	conn srt.Conn
	once sync.Once
	done chan struct{}
}

func (p *srtInput) Read(b []byte) (int, error) { return p.conn.Read(b) }
func (p *srtInput) Close() error {
	var e error
	p.once.Do(func() { close(p.done); e = p.conn.Close() })
	return e
}
func (s *Service) OpenSRT(ctx context.Context, id string) (io.ReadCloser, error) {
	if !s.allowed(id, "srt") {
		return nil, ErrDenied
	}
	snap := s.Snapshot()
	identity, e := s.credentials.Snapshot()
	if e != nil || snap.Catalog == nil || snap.Catalog.SRT == nil || snap.Config.validate(identity.Settings) != nil {
		return nil, ErrUnavailable
	}
	key, e := s.credentials.SRTSecret(snap.Catalog.PartnerID)
	if e != nil {
		return nil, ErrDenied
	}
	endpoint := net.JoinHostPort(snap.Config.TunnelIP, strconv.Itoa(snap.Catalog.SRT.Port))
	expected, e := netip.ParsePrefix(identity.Settings.Address)
	if e != nil || !srtRouteMatches(endpoint, identity.Settings.Interface, expected.Addr()) {
		return nil, ErrDenied
	}
	config := srt.DefaultConfig()
	config.Passphrase = key
	config.PBKeylen = 32
	config.EnforcedEncryption = true
	config.StreamId = snap.Catalog.PartnerID + ":" + id
	config.Latency = time.Duration(snap.Catalog.SRT.LatencyMS) * time.Millisecond
	config.ConnectionTimeout = 3 * time.Second
	config.PeerIdleTimeout = 5 * time.Second
	config.PayloadSize = 1316
	config.MSS = 1400
	config.MaxBW = 16 * 1024 * 1024
	config.ReceiverBufferSize = 8 * 1024 * 1024
	config.SendBufferSize = 8 * 1024 * 1024
	type result struct {
		conn srt.Conn
		err  error
	}
	completed := make(chan result)
	go func() {
		conn, e := srt.Dial("srt", endpoint, config)
		select {
		case completed <- result{conn: conn, err: e}:
		case <-ctx.Done():
			if conn != nil {
				conn.Close()
			}
		}
	}()
	var connected result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case connected = <-completed:
	}
	if connected.err != nil {
		return nil, ErrUnavailable
	}
	address, _, e := net.SplitHostPort(connected.conn.LocalAddr().String())
	if e != nil || address != expected.Addr().String() {
		connected.conn.Close()
		return nil, ErrDenied
	}
	input := &srtInput{conn: connected.conn, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			input.Close()
		case <-input.done:
		}
	}()
	return input, nil
}

// UDP connect performs route selection without transmitting a datagram. Reject
// a missing tunnel address or a different source before the SRT handshake starts.
// The resulting SRT socket is checked again; no route or NIC is changed here.
func srtRouteMatches(endpoint, name string, expected netip.Addr) bool {
	nic, e := net.InterfaceByName(name)
	if e != nil || nic.Flags&net.FlagUp == 0 {
		return false
	}
	addresses, e := nic.Addrs()
	if e != nil {
		return false
	}
	found := false
	for _, address := range addresses {
		prefix, e := netip.ParsePrefix(address.String())
		if e == nil && prefix.Addr() == expected {
			found = true
		}
	}
	if !found {
		return false
	}
	socket, e := net.DialTimeout("udp", endpoint, time.Second)
	if e != nil {
		return false
	}
	defer socket.Close()
	local, ok := socket.LocalAddr().(*net.UDPAddr)
	return ok && local.IP.String() == expected.String()
}
