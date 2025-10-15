package transport

import (
	"errors"
	"log/slog"
	"net"
	"sync"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"
)

type socket struct {
	conn4    *net.UDPConn
	conn6    *net.UDPConn
	connIPv4 *ipv4.PacketConn
	connIPv6 *ipv6.PacketConn

	ifaces       []net.Interface
	ifacesNoIPv4 map[int]struct{} // keyed by Interface.Index
	ifacesNoIPv6 map[int]struct{} // keyed by Interface.Index

	// Protect SetMulticastInterface + WriteToUDP as a single atomic operation
	// to avoid races when multicast is called concurrently from multiple goroutines.
	sendMu sync.Mutex

	closeOnce sync.Once
}

func newSocket(opts Options) (*socket, error) {
	s := &socket{
		ifaces:       opts.JoinIfaces,
		ifacesNoIPv4: make(map[int]struct{}),
		ifacesNoIPv6: make(map[int]struct{}),
	}

	addr4, addr6 := bindAddrs(opts.BindTo)

	var err4, err6 error
	if opts.IPVersion&IPv4 != 0 {
		err4 = s.newUDP4Conn(addr4)
	}
	if opts.IPVersion&IPv6 != 0 {
		err6 = s.newUDP6Conn(addr6)
	}

	if err4 != nil && err6 != nil {
		logger.Debug("failed to create both IPv4 and IPv6 socket", "err4", err4, "err6", err6)
		return nil, errors.Join(err4, err6)
	}

	if err4 != nil {
		logger.Warn("failed to create IPv4 socket", "err", err4)
	}
	if err6 != nil {
		logger.Warn("failed to create IPv6 socket", "err", err6)
	}

	logger.Debug("sockets created", slog.Bool("ipv4", s.conn4 != nil), slog.Bool("ipv6", s.conn6 != nil))

	return s, nil
}

func (s *socket) close() error {
	var err4, err6 error
	s.closeOnce.Do(func() {
		if s.conn4 != nil {
			// closing conn4 is sufficient to close connIPv4
			err4 = s.conn4.Close()
		}
		if s.conn6 != nil {
			// closing conn6 is sufficient to close connIPv6
			err6 = s.conn6.Close()
		}
	})
	return errors.Join(err4, err6)
}

func (s *socket) newUDP4Conn(addr *net.UDPAddr) error {
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return err
	}

	v4conn := ipv4.NewPacketConn(conn)
	s.connIPv4 = v4conn
	if err := v4conn.SetMulticastTTL(_MDNSDefaultHopLimit); err != nil {
		logger.Debug("failed to set multicast TTL on IPv4 socket; continuing", slog.Any("error", err))
	}
	if err := v4conn.SetMulticastLoopback(true); err != nil {
		logger.Debug("failed to set multicast loopback on IPv4 socket; continuing", slog.Any("error", err))
	}
	if err := v4conn.SetControlMessage(ipv4.FlagDst|ipv4.FlagInterface, true); err != nil {
		logger.Debug("failed to set control message on IPv4 socket; continuing", slog.Any("error", err))
	}

	var joined int

	for _, iface := range s.ifaces {
		if err := v4conn.JoinGroup(&iface, mdnsGaddrUDP4); err != nil {
			// silently ignore join errors for interfaces without IPv4 address
			hasIPv4, _, _ := interfaceIPVersion(&iface)
			if !hasIPv4 {
				s.ifacesNoIPv4[iface.Index] = struct{}{}
				continue
			}
			logger.Debug("failed to join ipv4 multicast group; skipping", slog.String("interface", iface.Name), slog.Any("error", err))
		} else {
			joined++
		}
	}

	if joined == 0 {
		return errors.New("no multicast group joined on any interface for IPv4")
	} else {
		logger.Debug("joined multicast group on IPv4 interfaces", slog.Int("joined", joined), slog.Int("total", len(s.ifaces)))
	}

	s.conn4 = conn
	return nil
}

func (s *socket) newUDP6Conn(addr *net.UDPAddr) error {
	conn, err := net.ListenUDP("udp6", addr)
	if err != nil {
		return err
	}

	v6conn := ipv6.NewPacketConn(conn)
	s.connIPv6 = v6conn
	if err := v6conn.SetMulticastHopLimit(_MDNSDefaultHopLimit); err != nil {
		logger.Debug("failed to set multicast hop limit on IPv6 socket; continuing", slog.Any("error", err))
	}
	if err := v6conn.SetMulticastLoopback(true); err != nil {
		logger.Debug("failed to set multicast loopback on IPv6 socket; continuing", slog.Any("error", err))
	}
	if err := v6conn.SetControlMessage(ipv6.FlagDst|ipv6.FlagInterface, true); err != nil {
		logger.Debug("failed to set control message on IPv6 socket; continuing", slog.Any("error", err))
	}

	var joined int

	for _, iface := range s.ifaces {
		if err := v6conn.JoinGroup(&iface, mdnsGaddrUDP6); err != nil {
			// silently ignore join errors for interfaces without IPv6 address
			_, hasIPv6, _ := interfaceIPVersion(&iface)
			if !hasIPv6 {
				s.ifacesNoIPv6[iface.Index] = struct{}{}
				continue
			}
			logger.Debug("failed to join ipv6 multicast group; skipping", slog.String("interface", iface.Name), slog.Any("error", err))
		} else {
			joined++
		}
	}

	if joined == 0 {
		return errors.New("no multicast group joined on any interface for IPv6")
	} else {
		logger.Debug("joined multicast group on IPv6 interfaces", slog.Int("joined", joined), slog.Int("total", len(s.ifaces)))
	}

	s.conn6 = conn
	return nil
}

func (s *socket) unicast(b []byte, addr *net.UDPAddr) error {
	var err error
	if addr.IP.To4() != nil {
		if s.conn4 == nil {
			return errors.New("no IPv4 socket available")
		}
		_, err = s.conn4.WriteToUDP(b, addr)
	} else if addr.IP.To16() != nil {
		if s.conn6 == nil {
			return errors.New("no IPv6 socket available")
		}
		_, err = s.conn6.WriteToUDP(b, addr)
	} else {
		return errors.New("address is not valid IPv4 or IPv6")
	}

	if err != nil {
		logger.Debug("failed to write to unicast address", slog.String("address", addr.String()), slog.Any("error", err))
		return err
	}

	logger.Debug("unicast message sent", slog.String("address", addr.String()))
	return nil
}

func (s *socket) multicast(b []byte) error {
	var sent4, sent6 int

	if s.conn4 != nil {
		for _, iface := range s.ifaces {
			if _, no := s.ifacesNoIPv4[iface.Index]; no {
				continue
			}
			s.sendMu.Lock()
			if err := s.connIPv4.SetMulticastInterface(&iface); err != nil {
				s.sendMu.Unlock()
				logger.Debug("failed to set multicast interface on IPv4 socket; skipping", slog.String("interface", iface.Name), slog.Any("error", err))
				continue
			}
			_, err := s.conn4.WriteToUDP(b, mdnsGaddrUDP4)
			s.sendMu.Unlock()
			if err != nil {
				logger.Debug("failed to write to IPv4 multicast address; skipping", slog.String("interface", iface.Name), slog.Any("error", err))
				continue
			}
			sent4++
		}
	}

	if s.conn6 != nil {
		for _, iface := range s.ifaces {
			if _, no := s.ifacesNoIPv6[iface.Index]; no {
				continue
			}
			s.sendMu.Lock()
			if err := s.connIPv6.SetMulticastInterface(&iface); err != nil {
				s.sendMu.Unlock()
				logger.Debug("failed to set multicast interface on IPv6 socket; skipping", slog.String("interface", iface.Name), slog.Any("error", err))
				continue
			}
			_, err := s.conn6.WriteToUDP(b, mdnsGaddrUDP6)
			s.sendMu.Unlock()
			if err != nil {
				logger.Debug("failed to write to IPv6 multicast address; skipping", slog.String("interface", iface.Name), slog.Any("error", err))
				continue
			}
			sent6++
		}
	}

	if sent4 == 0 && sent6 == 0 {
		return errors.New("no message sent on either IPv4 or IPv6")
	} else {
		logger.Debug("multicast message sent", slog.Int("sent4", sent4), slog.Int("sent6", sent6))
	}

	return nil
}
