package multicast

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"
)

func fmtPort(port int) string { return strconv.Itoa(port) }

const DatagramSize = 7 * 188

type Socket interface {
	Write([]byte) (int, error)
	SetWriteDeadline(time.Time) error
	Close() error
}
type Opener func(Target, int) (Socket, error)
type Stats struct {
	State           string `json:"state"`
	Bytes           uint64 `json:"bytes"`
	Datagrams       uint64 `json:"datagrams"`
	Errors          uint64 `json:"errors"`
	BufferedPackets int    `json:"buffered_packets"`
}
type Sender struct {
	mu     sync.Mutex
	ctx    context.Context
	conn   Socket
	buffer [DatagramSize]byte
	used   int
	closed bool
	stats  Stats
}

func New(ctx context.Context, t Target, interfaces []Interface, open Opener) (*Sender, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	index, e := resolve(t, interfaces)
	if e != nil {
		return nil, e
	}
	if open == nil {
		return nil, errors.New("socket opener unavailable")
	}
	conn, e := open(t, index)
	if e != nil {
		return nil, errors.New("multicast socket unavailable")
	}
	if ctx.Err() != nil {
		_ = conn.Close()
		return nil, ctx.Err()
	}
	return &Sender{ctx: ctx, conn: conn, stats: Stats{State: "ready"}}, nil
}
func (s *Sender) WritePacket(p []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.ctx.Err() != nil {
		return errors.New("multicast output closed")
	}
	if len(p) != 188 || p[0] != 0x47 {
		return errors.New("invalid TS packet")
	}
	copy(s.buffer[s.used:], p)
	s.used += len(p)
	s.stats.BufferedPackets = s.used / 188
	if s.used < DatagramSize {
		return nil
	}
	if s.conn.SetWriteDeadline(time.Now().Add(100*time.Millisecond)) != nil {
		return s.failed()
	}
	n, e := s.conn.Write(s.buffer[:])
	s.used = 0
	s.stats.BufferedPackets = 0
	if e != nil || n != DatagramSize {
		return s.failed()
	}
	s.stats.Bytes += uint64(n)
	s.stats.Datagrams++
	s.stats.State = "submitted_to_socket"
	return nil
}
func (s *Sender) failed() error {
	s.stats.Errors++
	s.stats.State = "failed"
	s.closed = true
	s.used = 0
	s.stats.BufferedPackets = 0
	_ = s.conn.Close()
	return errors.New("multicast write failed")
}
func (s *Sender) Snapshot() Stats { s.mu.Lock(); defer s.mu.Unlock(); return s.stats }
func (s *Sender) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	s.used = 0
	s.stats.BufferedPackets = 0
	s.stats.State = "closed"
	return s.conn.Close()
}

// Open validates against freshly discovered interfaces, never a default NIC.
func Open(ctx context.Context, t Target) (*Sender, error) {
	all, e := Interfaces()
	if e != nil {
		return nil, e
	}
	return New(ctx, t, all, openSocket)
}
