package simplemdns

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"sync"

	"github.com/miekg/dns"
)

type client struct {
	*sockets

	respHub *broadcaster[*dns.Msg]

	wg sync.WaitGroup
}

func NewClient() (*client, error) {
	s, err := newSockets(socketIPV4And6, socketBindZeroAddr, nil)
	if err != nil {
		return nil, err
	}

	c := &client{
		sockets: s,
		respHub: newBroadcaster[*dns.Msg](),
	}

	c.wg.Go(c.receiving)

	return c, nil
}

func (c *client) Close() error {
	c.respHub.close()
	err := c.sockets.close()
	c.wg.Wait()
	return err
}

func (c *client) Responses() <-chan *dns.Msg {
	return c.respHub.subscribe()
}

func (c *client) SendQuery(msg *dns.Msg) (err error) {
	packed, err := msg.Pack()
	if err != nil {
		return
	}

	logger.Debug("sending DNS message", slog.Int("questions", len(msg.Question)))
	err = c.sockets.send(packed)
	return
}

func (c *client) QueryOneShot(ctx context.Context, question dns.Question) (dns.RR, error) {
	msg := new(dns.Msg)
	msg.Question = []dns.Question{question}

	respCh := c.respHub.subscribe()

	if err := c.SendQuery(msg); err != nil {
		return nil, err
	}

	for {
		select {
		case resp, ok := <-respCh:
			if !ok {
				return nil, errors.New("client closed")
			}

			for _, ans := range resp.Answer {
				if ans.Header().Name == question.Name &&
					ans.Header().Rrtype == question.Qtype &&
					ans.Header().Class == question.Qclass {
					return ans, nil
				}
			}
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (c *client) receiving() {
	buf := make([]byte, udpBufSize)

	for {
		n, err := c.sockets.receive(buf)
		if errors.Is(err, net.ErrClosed) {
			return
		}
		if err != nil {
			logger.Warn("error reading from UDP socket", slog.Any("error", err))
			continue
		}

		var msg dns.Msg
		if err := msg.Unpack(buf[:n]); err != nil {
			logger.Warn("error unpacking DNS message", slog.Any("error", err))
			continue
		}

		logger.Debug("received DNS message", slog.Int("answers", len(msg.Answer)))
		c.respHub.broadcast(&msg)
	}
}
