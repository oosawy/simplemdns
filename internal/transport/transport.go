package transport

import (
	"log/slog"
	"net"

	"github.com/miekg/dns"
)

// TODO: replace this with a more flexible logging solution
var logger = slog.Default().With("lib", "simplemdns")

// Transport is a minimal interface for mDNS transport.
type Transport interface {
	Messages() <-chan *dns.Msg
	SendMsg(*dns.Msg) error
	SendMsgTo(*dns.Msg, *net.UDPAddr) error
	Close() error
}

// New creates a transport with given options. Minimal placeholder; implementation
// will live in conn.go.
func New(opts Options) (Transport, error) {
	o, err := opts.withDefaults()
	if err != nil {
		return nil, err
	}

	return newConn(o)
}
