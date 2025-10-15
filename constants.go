package simplemdns

import (
	"github.com/oosawy/simplemdns/internal/transport"
)

const (
	// IPv4 specifies to use only IPv4 for mDNS communication.
	IPv4 = transport.IPv4
	// IPv6 specifies to use only IPv6 for mDNS communication.
	IPv6 = transport.IPv6
	// IPv4AndIPv6 specifies to use both IPv4 and IPv6 for mDNS communication.
	IPv4AndIPv6 = transport.IPv4And6
)

const (
	// BindZeroAddr binds to the zero address and port.
	BindZeroAddr = transport.BindZeroAddr // i.e. 0.0.0.0:0
	// BindMDNSPort binds to the mDNS port on all interfaces.
	BindMDNSPort = transport.BindMDNSPort // i.e. 0.0.0.0:5353
	// BindMDNSGaddr binds to the mDNS multicast group address.
	BindMDNSGaddr = transport.BindMDNSGaddr // i.e. 224.0.0.251:5353
)
