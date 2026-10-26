package security

import (
	"net"
	"sync"
)

type limitedListener struct {
	net.Listener
	slots chan struct{}
	done  chan struct{}
	once  sync.Once
}
type limitedConn struct {
	net.Conn
	release func()
	once    sync.Once
}

func (c *limitedConn) Close() error { e := c.Conn.Close(); c.once.Do(c.release); return e }
func LimitListener(parent net.Listener) net.Listener {
	return &limitedListener{Listener: parent, slots: make(chan struct{}, 64), done: make(chan struct{})}
}
func (l *limitedListener) Accept() (net.Conn, error) {
	select {
	case <-l.done:
		return nil, net.ErrClosed
	case l.slots <- struct{}{}:
	}
	conn, e := l.Listener.Accept()
	if e != nil {
		<-l.slots
		return nil, e
	}
	return &limitedConn{Conn: conn, release: func() { <-l.slots }}, nil
}
func (l *limitedListener) Close() error {
	l.once.Do(func() { close(l.done) })
	return l.Listener.Close()
}
