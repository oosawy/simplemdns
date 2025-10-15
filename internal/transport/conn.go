package transport

import (
	"net"
	"sync"

	"github.com/miekg/dns"
)

type mdnsConn struct {
	*socket

	msgs chan *dns.Msg

	wg        sync.WaitGroup
	closeOnce sync.Once
}

func newConn(opts Options) (Transport, error) {
	socket, err := newSocket(opts)
	if err != nil {
		return nil, err
	}

	c := &mdnsConn{
		socket: socket,
		msgs:   make(chan *dns.Msg, opts.MsgsChBufSize),
	}

	c.startRecvLoop(opts.UDPRecvBufSize)

	return c, nil
}

func (c *mdnsConn) Close() (err error) {
	c.closeOnce.Do(func() {
		err = c.socket.close()
		c.wg.Wait()
		close(c.msgs)
	})
	return
}

func (c *mdnsConn) send(b []byte) error {
	return c.socket.multicast(b)
}

func (c *mdnsConn) sendTo(b []byte, addr *net.UDPAddr) error {
	return c.socket.unicast(b, addr)
}
