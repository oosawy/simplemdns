package simplemdns

import (
	"context"
	"errors"
	"sync"

	"github.com/miekg/dns"
)

type client struct {
	*mdnsConn

	respHub *broadcaster[*dns.Msg]

	wg sync.WaitGroup
}

func NewClient() (*client, error) {
	s, err := newMDNSConn(socketIPV4And6, socketBindZeroAddr, nil)
	if err != nil {
		return nil, err
	}

	c := &client{
		mdnsConn: s,
		respHub:  newBroadcaster[*dns.Msg](),
	}

	c.wg.Go(c.receiving)

	return c, nil
}

func (c *client) Close() error {
	c.respHub.close()
	err := c.mdnsConn.close()
	c.wg.Wait()
	return err
}

func (c *client) Responses() <-chan *dns.Msg {
	return c.respHub.subscribe()
}

func (c *client) SendQuery(msg *dns.Msg) (err error) {
	return c.mdnsConn.sendMsg(msg)
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
	msgs := c.mdnsConn.messages()
	for {
		msg, ok := <-msgs
		if !ok {
			return
		}
		c.respHub.broadcast(msg)
	}
}
