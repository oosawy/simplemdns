package transport

import "net"

const _MDNSDefaultHopLimit = 255

var (
	mdnsGaddrIPV4 = net.IPv4(224, 0, 0, 251)
	mdnsGaddrIPV6 = net.ParseIP("ff02::fb")
	mdnsPort      = 5353

	mdnsGaddrUDP4 = &net.UDPAddr{IP: mdnsGaddrIPV4, Port: mdnsPort}
	mdnsGaddrUDP6 = &net.UDPAddr{IP: mdnsGaddrIPV6, Port: mdnsPort}

	zeroAddrUDP4 = &net.UDPAddr{IP: net.IPv4zero, Port: 0}
	zeroAddrUDP6 = &net.UDPAddr{IP: net.IPv6unspecified, Port: 0}

	mdnsZeroAddrUDP4 = &net.UDPAddr{IP: net.IPv4zero, Port: mdnsPort}
	mdnsZeroAddrUDP6 = &net.UDPAddr{IP: net.IPv6unspecified, Port: mdnsPort}
)

type IPVersion int

const (
	IPv4     IPVersion     = 1 << iota // 0b01
	IPv6                               // 0b10
	IPv4And6 = IPv4 | IPv6             // 0b11
)

type BindStrategy int

const (
	BindZeroAddr BindStrategy = iota + 1
	BindMDNSPort
	BindMDNSGaddr
)

func bindAddrs(strategy BindStrategy) (udp4addr, udp6addr *net.UDPAddr) {
	switch strategy {
	case BindZeroAddr:
		udp4addr = zeroAddrUDP4
		udp6addr = zeroAddrUDP6
	case BindMDNSPort:
		udp4addr = mdnsZeroAddrUDP4
		udp6addr = mdnsZeroAddrUDP6
	case BindMDNSGaddr:
		udp4addr = mdnsGaddrUDP4
		udp6addr = mdnsGaddrUDP6
	}
	return
}
