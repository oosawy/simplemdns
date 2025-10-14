package simplemdns

import (
	"errors"
	"log/slog"
	"net"
	"sync"

	"github.com/miekg/dns"
)

type mdnsConn struct {
	*sockets

	wg        sync.WaitGroup
	conn4     *net.UDPConn
	conn6     *net.UDPConn
	msgCh     chan *dns.Msg
	closeOnce sync.Once
}

func newMDNSConn(ip socketIPVersion, bind socketBindStrategy, ifaces []net.Interface) (*mdnsConn, error) {
	mconn := &mdnsConn{}

	socks, err := newSockets(ip, bind, ifaces)
	if err != nil {
		return nil, err
	}
	mconn.sockets = socks

	mconn.msgCh = make(chan *dns.Msg, 16)

	if socks.conn4 != nil {
		mconn.wg.Add(1)
		go mconn.recvLoop(socks.conn4)
	}
	if socks.conn6 != nil {
		mconn.wg.Add(1)
		go mconn.recvLoop(socks.conn6)
	}

	return mconn, nil
}

func (mconn *mdnsConn) close() (err error) {
	mconn.closeOnce.Do(func() {
		mconn.sockets.close()
		mconn.wg.Wait()
		close(mconn.msgCh)
	})

	return nil
}

func (mconn *mdnsConn) recvLoop(conn *net.UDPConn) {
	defer mconn.wg.Done()
	buf := make([]byte, udpBufSize)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			logger.Warn("error receiving UDP message", slog.Any("error", err))
			return
		}

		data := make([]byte, n)
		copy(data, buf[:n])

		var msg dns.Msg
		if err := msg.Unpack(data); err != nil {
			logger.Warn("error unpacking DNS message", slog.Any("error", err))
			continue
		}

		mconn.msgCh <- &msg
	}
}

func (mconn *mdnsConn) messages() <-chan *dns.Msg {
	return mconn.msgCh
}

func (mconn *mdnsConn) sendPacked(buf []byte) error {
	var err4, err6 error
	// TODO: send multicast
	if mconn.conn4 != nil {
		_, err4 = mconn.conn4.WriteToUDP(buf, mdnsGaddrUDP4)
	}
	if mconn.conn6 != nil {
		_, err6 = mconn.conn6.WriteToUDP(buf, mdnsGaddrUDP6)
	}
	if err4 != nil && err6 != nil {
		return errors.Join(err4, err6)
	}
	if err4 != nil {
		logger.Warn("error sending DNS message via udp4", slog.Any("error", err4))
	}
	if err6 != nil {
		logger.Warn("error sending DNS message via udp6", slog.Any("error", err6))
	}
	return nil
}

func (mconn *mdnsConn) sendMsg(m *dns.Msg) error {
	packed, err := m.Pack()
	if err != nil {
		return err
	}
	return mconn.sendPacked(packed)
}
