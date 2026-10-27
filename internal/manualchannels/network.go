package manualchannels

import (
	"context"
	srt "github.com/datarhei/gosrt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"sync"
	"time"
)

func lookupDefault(ctx context.Context, name string) ([]net.IPAddr, error) {
	return net.DefaultResolver.LookupIPAddr(ctx, name)
}
func permitted(ip netip.Addr) bool {
	if !ip.IsValid() || ip.Is4In6() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.Zone() != "" {
		return false
	}
	for _, prefix := range []string{"100.64.0.0/10", "0.0.0.0/8", "240.0.0.0/4", "fd00:ec2::254/128"} {
		if netip.MustParsePrefix(prefix).Contains(ip) {
			return false
		}
	}
	return true
}
func (s *Service) resolve(ctx context.Context, host string) (string, error) {
	addresses, e := s.lookup(ctx, host)
	if e != nil || len(addresses) == 0 || len(addresses) > 32 {
		return "", ErrUnavailable
	}
	for _, a := range addresses {
		ip, ok := netip.AddrFromSlice(a.IP)
		if !ok || !permitted(ip.Unmap()) {
			return "", ErrDenied
		}
	}
	return addresses[0].IP.String(), nil
}
func (s *Service) httpClient() *http.Client {
	t := &http.Transport{Proxy: nil, MaxIdleConns: 16, MaxIdleConnsPerHost: 4, MaxConnsPerHost: 64, IdleConnTimeout: 20 * time.Second, ResponseHeaderTimeout: 4 * time.Second, TLSHandshakeTimeout: 3 * time.Second, DialContext: func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, e := net.SplitHostPort(address)
		if e != nil {
			return nil, ErrInvalid
		}
		ip, e := s.resolve(ctx, host)
		if e != nil {
			return nil, e
		}
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip, port))
	}}
	return &http.Client{Transport: t, CheckRedirect: func(*http.Request, []*http.Request) error { return ErrDenied }}
}
func (s *Service) openHTTP(ctx context.Context, raw, rangeValue string) (*http.Response, error) {
	u, e := parse(raw)
	if e != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, ErrInvalid
	}
	request, e := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if e != nil {
		return nil, ErrInvalid
	}
	request.Header.Set("Accept", "*/*")
	if rangeValue != "" {
		request.Header.Set("Range", rangeValue)
	}
	response, e := s.client.Do(request)
	if e != nil {
		return nil, ErrUnavailable
	}
	if response.StatusCode != 200 && response.StatusCode != 206 {
		response.Body.Close()
		return nil, ErrUnavailable
	}
	if strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "text/html") || strings.HasPrefix(strings.ToLower(response.Header.Get("Content-Type")), "application/json") {
		response.Body.Close()
		return nil, ErrInvalid
	}
	return response, nil
}

type cancelReader struct {
	io.ReadCloser
	done chan struct{}
	once sync.Once
}

func (r *cancelReader) Close() error {
	var e error
	r.once.Do(func() { close(r.done); e = r.ReadCloser.Close() })
	return e
}
func contextual(ctx context.Context, input io.ReadCloser) io.ReadCloser {
	r := &cancelReader{ReadCloser: input, done: make(chan struct{})}
	go func() {
		select {
		case <-ctx.Done():
			r.Close()
		case <-r.done:
		}
	}()
	return r
}
func (s *Service) openSRT(ctx context.Context, c Source) (io.ReadCloser, error) {
	u, e := parse(c.URL)
	if e != nil {
		return nil, e
	}
	ip, e := s.resolve(ctx, u.Hostname())
	if e != nil {
		return nil, e
	}
	q := u.Query()
	config := srt.DefaultConfig()
	config.StreamId = q.Get("streamid")
	config.Passphrase = q.Get("passphrase")
	config.EnforcedEncryption = config.Passphrase != ""
	config.PayloadSize = 1316
	config.MSS = 1400
	config.ConnectionTimeout = 3 * time.Second
	config.PeerIdleTimeout = 5 * time.Second
	config.MaxBW = 16 * 1024 * 1024
	config.ReceiverBufferSize = 8 * 1024 * 1024
	config.SendBufferSize = 8 * 1024 * 1024
	if config.Passphrase != "" {
		config.PBKeylen = 32
	}
	if v := q.Get("pbkeylen"); v != "" {
		config.PBKeylen, _ = strconv.Atoi(v)
	}
	config.Latency = -1
	config.ReceiverLatency = 200 * time.Millisecond
	config.PeerLatency = 200 * time.Millisecond
	for _, key := range []string{"latency", "rcvlatency", "peerlatency"} {
		if v := q.Get(key); v != "" {
			n, _ := strconv.Atoi(v)
			d := time.Duration(n) * time.Microsecond
			switch key {
			case "latency":
				config.ReceiverLatency = d
				config.PeerLatency = d
			case "rcvlatency":
				config.ReceiverLatency = d
			case "peerlatency":
				config.PeerLatency = d
			}
		}
	}
	type result struct {
		conn srt.Conn
		err  error
	}
	ready := make(chan result)
	go func() {
		conn, e := s.dialSRT("srt", net.JoinHostPort(ip, u.Port()), config)
		select {
		case ready <- result{conn, e}:
		case <-ctx.Done():
			if conn != nil {
				conn.Close()
			}
		}
	}()
	select {
	case <-ctx.Done():
		return nil, ErrUnavailable
	case r := <-ready:
		if r.err != nil {
			return nil, ErrUnavailable
		}
		return contextual(ctx, r.conn), nil
	}
}
