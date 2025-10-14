package simplemdns

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/miekg/dns"
)

type client struct {
	*mdnsConn

	respHub *broadcaster[*dns.Msg]

	wg sync.WaitGroup
}

type ClientOptions struct {
	Interfaces   []net.Interface
	BindStrategy socketBindStrategy
	IPVersion    socketIPVersion
}

func (o *ClientOptions) withDefaults() *ClientOptions {
	if o == nil {
		return &ClientOptions{
			BindStrategy: socketBindZeroAddr,
			IPVersion:    socketIPV4And6,
			Interfaces:   nil,
		}
	}

	c := *o
	if c.BindStrategy == 0 {
		c.BindStrategy = socketBindZeroAddr
	}
	if c.IPVersion == 0 {
		c.IPVersion = socketIPV4And6
	}
	return &c
}

func NewClient() (*client, error) {
	return NewClientWithOptions(nil)
}

func NewClientWithOptions(opts *ClientOptions) (*client, error) {
	o := opts.withDefaults()

	s, err := newMDNSConn(o.IPVersion, o.BindStrategy, o.Interfaces)
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
