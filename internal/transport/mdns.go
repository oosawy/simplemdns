package transport

import (
	"errors"
	"log/slog"
	"net"

	"github.com/miekg/dns"
)

func (c *mdnsConn) Messages() <-chan *dns.Msg {
	return c.msgs
}

func (c *mdnsConn) SendMsg(msg *dns.Msg) error {
	defer logger.Debug("sent DNS message",
		slog.Int("questions", len(msg.Question)),
		slog.Int("answers", len(msg.Answer)),
		slog.Any("names", msgNames(msg)))

	b, err := msg.Pack()
	if err != nil {
		return err
	}
	return c.send(b)
}

func (c *mdnsConn) SendMsgTo(msg *dns.Msg, addr *net.UDPAddr) error {
	defer logger.Debug("sent DNS message",
		slog.String("to", addr.String()),
		slog.Int("questions", len(msg.Question)),
		slog.Int("answers", len(msg.Answer)),
		slog.Any("names", msgNames(msg)))

	b, err := msg.Pack()
	if err != nil {
		return err
	}
	return c.sendTo(b, addr)
}

func (c *mdnsConn) startRecvLoop(bufSize int) {
	if c.conn4 != nil {
		c.wg.Go(func() {
			recvLoop(c.conn4, c.msgs, bufSize)
		})
	}
	if c.conn6 != nil {
		c.wg.Go(func() {
			recvLoop(c.conn6, c.msgs, bufSize)
		})
	}
}

func recvLoop(conn *net.UDPConn, msgCh chan<- *dns.Msg, bufSize int) {
	buf := make([]byte, bufSize)
	for {
		n, from, err := conn.ReadFromUDP(buf)
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			logger.Warn("error receiving UDP message", slog.Any("error", err))
			continue
		}

		msg := new(dns.Msg)
		if err := msg.Unpack(buf[:n]); err != nil {
			logger.Warn("error unpacking DNS message", slog.Any("error", err))
			continue
		}

		logger.Debug("received DNS message",
			slog.String("from", from.String()),
			slog.Int("questions", len(msg.Question)),
			slog.Int("answers", len(msg.Answer)),
			slog.Any("names", msgNames(msg)))

		select {
		case msgCh <- msg:
		default:
			logger.Debug("dropping DNS message due to full channel")
		}
	}
}

func msgNames(m *dns.Msg) []string {
	names := make(map[string]struct{})
	for _, q := range m.Question {
		names[q.Name] = struct{}{}
	}
	for _, rr := range m.Answer {
		names[rr.Header().Name] = struct{}{}
	}
	for _, rr := range m.Ns {
		names[rr.Header().Name] = struct{}{}
	}
	for _, rr := range m.Extra {
		names[rr.Header().Name] = struct{}{}
	}
	var uniq []string
	for name := range names {
		uniq = append(uniq, name)
	}
	return uniq
}
