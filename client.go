package simplemdns

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/miekg/dns"

	"github.com/oosawy/simplemdns/internal/transport"
)

// ClientOptions controls how the client creates its transport.
type ClientOptions struct {
	IPVersion      transport.IPVersion
	BindTo         transport.BindStrategy
	Interfaces     []net.Interface // nil or empty for all available multicast interfaces
	UDPRecvBufSize int             // in bytes; should be at least 1500; will be set to 1500 if less
	MsgsChBufSize  int             // msgs drop when full
}

func (o ClientOptions) withDefaults() ClientOptions {
	if o.IPVersion == 0 {
		o.IPVersion = transport.IPv4And6
	}
	if o.BindTo == 0 {
		// TODO: currently, works as simple resolver by default.
		o.BindTo = transport.BindZeroAddr
	}
	if o.UDPRecvBufSize == 0 {
		o.UDPRecvBufSize = 1500 // is the typical MTU of Ethernet minus some overhead.
	}
	if o.MsgsChBufSize == 0 {
		// If MsgsChBufSize is full, incoming packets are dropped.
		// mDNS runs over UDP and uses retries to handle packet loss.
		// However, some networks may produce bursts of responses (often a dozen or more at once).
		// To efficiently absorb these bursts and avoid dropping messages,
		// we use 32 as the default buffer size.
		o.MsgsChBufSize = 32
	}

	if o.UDPRecvBufSize < 1500 {
		o.UDPRecvBufSize = 1500
	}

	return o
}

type client struct {
	t transport.Transport

	closeOnce sync.Once

	subscribers     []chan *dns.Msg
	subMu           sync.Mutex
	broadcasterOnce sync.Once
}

// NewClient creates a new client using provided ClientOptions. Accepts zero or
// one ClientOptions. If opts is nil, sensible defaults are used.
// In common use cases, you don't need to provide any options.
func NewClient(opts ...ClientOptions) (*client, error) {
	var o ClientOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	o = o.withDefaults()

	t, err := transport.New(transport.Options{
		IPVersion:      o.IPVersion,
		BindTo:         o.BindTo,
		JoinIfaces:     o.Interfaces,
		UDPRecvBufSize: o.UDPRecvBufSize,
		MsgsChBufSize:  o.MsgsChBufSize,
	})
	if err != nil {
		return nil, err
	}

	return &client{t: t}, nil
}

func (c *client) Close() (err error) {
	c.closeOnce.Do(func() {
		err = c.t.Close()

		c.subMu.Lock()
		for _, sub := range c.subscribers {
			close(sub)
		}
		c.subscribers = nil
		c.subMu.Unlock()
	})
	return
}

// Subscribe returns a new subscriber channel that will be closed when the client is closed.
func (c *client) Subscribe() <-chan *dns.Msg {
	ch := make(chan *dns.Msg, 32)

	c.subMu.Lock()
	c.subscribers = append(c.subscribers, ch)
	c.subMu.Unlock()

	c.broadcasterOnce.Do(func() {
		go func() {
			for msg := range c.t.Messages() {
				c.subMu.Lock()
				subs := make([]chan *dns.Msg, len(c.subscribers))
				copy(subs, c.subscribers)
				c.subMu.Unlock()
				for _, sub := range subs {
					select {
					case sub <- msg:
					default:
						// drop if subscriber channel is full
					}
				}
			}
			// when t.Messages() is closed, close all subscribers
			c.subMu.Lock()
			for _, sub := range c.subscribers {
				close(sub)
			}
			c.subscribers = nil
			c.subMu.Unlock()
		}()
	})

	return ch
}

// TODO: accept ch to send responses, and a context to cancel
// Query sends a dns.Msg via the transport.
func (c *client) Query(msg *dns.Msg) error {
	return c.t.SendMsg(msg)
}

// QueryFirst sends a query and waits for the first matching answer.
// Note: This method behaves like an RFC one-shot query, but uses mDNS (multicast)
// rather than unicast. It exists for convenience and may be deprecated in the future.
func (c *client) QueryFirst(ctx context.Context, question dns.Question) (dns.RR, error) {
	msg := new(dns.Msg)
	msg.Question = []dns.Question{question}

	msgCh := c.Subscribe()

	if err := c.Query(msg); err != nil {
		return nil, err
	}

	for {
		select {
		case resp, ok := <-msgCh:
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
