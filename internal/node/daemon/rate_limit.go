package daemon

import (
	"net"
	"sync"
)

type rateLimitedListener struct {
	net.Listener
	Pool chan struct{}
}

func (l *rateLimitedListener) Accept() (net.Conn, error) {
	<-l.Pool

	conn, err := l.Listener.Accept()
	if err != nil {
		l.Pool <- struct{}{}
		return nil, err
	}

	return &rateLimitedConn{Conn: conn, pool: l.Pool}, nil
}

type rateLimitedConn struct {
	net.Conn
	once sync.Once
	pool chan<- struct{}
}

func (c *rateLimitedConn) release() {
	c.once.Do(func() {
		c.pool <- struct{}{}
	})
}

func (c *rateLimitedConn) didClose(err error) {
	if err == nil {
		return
	}

	// False positives could allow more connections than the limit. False
	// negatives could cause the pool to leak. In the extreme this would prevent
	// any new connections.
	netErr, ok := err.(net.Error)
	if !ok || !netErr.Timeout() {
		c.release()
	}
}

func (c *rateLimitedConn) Close() error {
	c.release()
	return c.Conn.Close()
}

func (c *rateLimitedConn) Read(b []byte) (n int, err error) {
	n, err = c.Conn.Read(b)
	c.didClose(err)
	return n, err
}

func (c *rateLimitedConn) Write(b []byte) (n int, err error) {
	n, err = c.Conn.Write(b)
	c.didClose(err)
	return n, err
}
