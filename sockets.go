package simplemdns

import (
	"errors"
	"log/slog"
	"net"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

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

type socketIPVersion int

const (
	socketIPV4 socketIPVersion = 1 << iota
	socketIPV6
	socketIPV4And6 = socketIPV4 | socketIPV6
)

func (s socketIPVersion) String() string {
	switch s {
	case socketIPV4:
		return "ipv4"
	case socketIPV6:
		return "ipv6"
	case socketIPV4And6:
		return "ipv4/ipv6"
	default:
		return "unknown"
	}
}

type socketBindStrategy int

const (
	socketBindZeroAddr socketBindStrategy = iota + 1
	socketBindMDNSPort
	socketBindMDNSGaddr
)

type sockets struct {
	conn4         *net.UDPConn
	conn6         *net.UDPConn
	ifaces        []net.Interface
	ifaceVersions map[int]socketIPVersion // keyed by Interface.Index
}

func socketAddrs(strategy socketBindStrategy) (udp4addr, udp6addr *net.UDPAddr) {
	switch strategy {
	case socketBindZeroAddr:
		udp4addr = zeroAddrUDP4
		udp6addr = zeroAddrUDP6
	case socketBindMDNSPort:
		udp4addr = mdnsZeroAddrUDP4
		udp6addr = mdnsZeroAddrUDP6
	case socketBindMDNSGaddr:
		udp4addr = mdnsGaddrUDP4
		udp6addr = mdnsGaddrUDP6
	}
	return
}

func interfaces() ([]net.Interface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	mifaces := make([]net.Interface, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagMulticast == 0 {
			continue
		}
		mifaces = append(mifaces, iface)
	}
	return mifaces, nil
}

func ifaceIPVersion(iface net.Interface) (hasIPv4, hasIPv6 bool, err error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return false, false, err
	}
	for _, a := range addrs {
		var ip net.IP
		switch v := a.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			continue
		}
		if ip == nil {
			continue
		}
		if ip.To4() != nil {
			hasIPv4 = true
		} else if ip.To16() != nil {
			hasIPv6 = true
		}
		if hasIPv4 && hasIPv6 {
			return true, true, nil
		}
	}
	return hasIPv4, hasIPv6, nil
}

func (s *sockets) ifaceSupports(iface *net.Interface, want socketIPVersion) bool {
	if s == nil || s.ifaceVersions == nil {
		return true
	}
	if v, ok := s.ifaceVersions[iface.Index]; ok {
		return v&want != 0
	}
	return true
}

func newSockets(ip socketIPVersion, bind socketBindStrategy, ifaces []net.Interface) (*sockets, error) {
	s := &sockets{}

	udp4addr, udp6addr := socketAddrs(bind)

	s.ifaces = ifaces
	if s.ifaces == nil {
		var err error
		s.ifaces, err = interfaces()
		if err != nil {
			return nil, err
		}
	}

	s.ifaceVersions = make(map[int]socketIPVersion, len(s.ifaces))
	for _, iface := range s.ifaces {
		has4, has6, _ := ifaceIPVersion(iface)
		var v socketIPVersion
		if has4 {
			v |= socketIPV4
		}
		if has6 {
			v |= socketIPV6
		}
		s.ifaceVersions[iface.Index] = v
	}

	var err4, err6 error
	if ip&socketIPV4 != 0 {
		s.conn4, err4 = net.ListenUDP("udp4", udp4addr)
	}
	if ip&socketIPV6 != 0 {
		s.conn6, err6 = net.ListenUDP("udp6", udp6addr)
	}

	// initialize connections (use cached iface info inside initConn)
	if s.conn4 != nil {
		initConn(s, socketIPV4, s.conn4)
	}
	if s.conn6 != nil {
		initConn(s, socketIPV6, s.conn6)
	}

	if err4 != nil && err6 != nil {
		return nil, errors.Join(err4, err6)
	}
	if err4 != nil {
		logger.Debug("failed to create udp4 socket; using udp6 only", slog.Any("error", err4))
	}
	if err6 != nil {
		logger.Debug("failed to create udp6 socket; using udp4 only", slog.Any("error", err6))
	}

	return s, nil
}

func initConn(s *sockets, ip socketIPVersion, conn *net.UDPConn) {
	var joined int

	switch ip {
	case socketIPV4:
		v4conn := ipv4.NewPacketConn(conn)
		if err := v4conn.SetMulticastTTL(255); err != nil {
			logger.Debug("failed to set multicast TTL", slog.Any("error", err))
		}
		if err := v4conn.SetMulticastLoopback(true); err != nil {
			logger.Debug("failed to set multicast loopback", slog.Any("error", err))
		}
		for i := range s.ifaces {
			iface := &s.ifaces[i]
			if !s.ifaceSupports(iface, socketIPV4) {
				continue
			}
			if err := v4conn.JoinGroup(iface, mdnsGaddrUDP4); err != nil {
				logger.Debug("failed to join ipv4 multicast group", slog.String("interface", iface.Name), slog.Any("error", err))
			} else {
				joined++
			}
		}
	case socketIPV6:
		v6conn := ipv6.NewPacketConn(conn)
		if err := v6conn.SetMulticastHopLimit(255); err != nil {
			logger.Debug("failed to set multicast hop limit", slog.Any("error", err))
		}
		if err := v6conn.SetMulticastLoopback(true); err != nil {
			logger.Debug("failed to set multicast loopback", slog.Any("error", err))
		}
		for i := range s.ifaces {
			iface := &s.ifaces[i]
			if !s.ifaceSupports(iface, socketIPV6) {
				continue
			}
			if err := v6conn.JoinGroup(iface, mdnsGaddrUDP6); err != nil {
				logger.Debug("failed to join ipv6 multicast group", slog.String("interface", iface.Name), slog.Any("error", err))
			} else {
				joined++
			}
		}
	}

	if joined == 0 {
		logger.Warn("no multicast group joined; mDNS may not work")
	} else {
		logger.Debug("joined multicast groups",
			slog.String("ip", ip.String()),
			slog.Int("joined", joined),
			slog.Int("all", len(s.ifaces)))
	}
}

func (s *sockets) close() error {
	if s.conn4 != nil {
		s.conn4.Close()
		s.conn4 = nil
	}
	if s.conn6 != nil {
		s.conn6.Close()
		s.conn6 = nil
	}
	return nil
}

func (s *sockets) multicast(b []byte) (err error) {
	if s.conn4 != nil {
		for iface := range s.ifaces {
			v4conn := ipv4.NewPacketConn(s.conn4)
			if !s.ifaceSupports(&s.ifaces[iface], socketIPV4) {
				continue
			}
			if err := v4conn.SetMulticastInterface(&s.ifaces[iface]); err != nil {
				logger.Debug("failed to set multicast interface for udp4", slog.String("interface", s.ifaces[iface].Name), slog.Any("error", err))
				continue
			}
			_, err := s.conn4.WriteToUDP(b, mdnsGaddrUDP4)
			if err != nil {
				logger.Warn("error sending DNS message via udp4", slog.String("interface", s.ifaces[iface].Name), slog.Any("error", err))
			}
		}
	}

	if s.conn6 != nil {
		for i := range s.ifaces {
			iface := &s.ifaces[i]
			if !s.ifaceSupports(iface, socketIPV6) {
				continue
			}
			v6conn := ipv6.NewPacketConn(s.conn6)
			if err := v6conn.SetMulticastInterface(iface); err != nil {
				logger.Debug("failed to set multicast interface for udp6", slog.String("interface", iface.Name), slog.Any("error", err))
				continue
			}
			_, err := s.conn6.WriteToUDP(b, mdnsGaddrUDP6)
			if err != nil {
				logger.Warn("error sending DNS message via udp6", slog.String("interface", iface.Name), slog.Any("error", err))
			}
		}
	}

	return nil
}
