package simplemdns

import (
	"errors"
	"log/slog"
	"net"
	"sync"

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
)

type socketIPVersion int

const (
	socketIPV4 socketIPVersion = 1 << iota
	socketIPV6
	socketIPV4And6 = socketIPV4 | socketIPV6
)

type socketBindStrategy int

const (
	socketBindZeroAddr socketBindStrategy = iota
	socketBindMDNSPort
	socketBindMDNSGaddr
)

type sockets struct {
	wg       sync.WaitGroup
	conn4    *net.UDPConn
	conn6    *net.UDPConn
	recvCh   chan []byte
	recvOnce sync.Once
}

func socketAddrs(strategy socketBindStrategy) (udp4addr, udp6addr *net.UDPAddr) {
	switch strategy {
	case socketBindZeroAddr:
		udp4addr = zeroAddrUDP4
		udp6addr = zeroAddrUDP6
	case socketBindMDNSPort:
		udp4addr = &net.UDPAddr{IP: net.IPv4zero, Port: mdnsPort}
		udp6addr = &net.UDPAddr{IP: net.IPv6unspecified, Port: mdnsPort}
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

func newSockets(ip socketIPVersion, bind socketBindStrategy, ifaces []net.Interface) (*sockets, error) {
	s := &sockets{}

	udp4addr, upd6addr := socketAddrs(bind)

	if ifaces == nil {
		var err error
		ifaces, err = interfaces()
		if err != nil {
			return nil, err
		}
	}

	var err4, err6 error

	if ip&socketIPV4 != 0 {
		s.conn4, err4 = net.ListenUDP("udp4", udp4addr)
		if err4 == nil {
			initConn(socketIPV4, s.conn4, ifaces)
		}
	}
	if ip&socketIPV6 != 0 {
		s.conn6, err6 = net.ListenUDP("udp6", upd6addr)
		if err6 == nil {
			initConn(socketIPV6, s.conn6, ifaces)
		}
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

func initConn(ip socketIPVersion, conn *net.UDPConn, ifaces []net.Interface) {
	var joined bool

	switch ip {
	case socketIPV4:
		v4conn := ipv4.NewPacketConn(conn)
		if err := v4conn.SetMulticastTTL(255); err != nil {
			logger.Debug("failed to set multicast TTL", slog.Any("error", err))
		}
		if err := v4conn.SetMulticastLoopback(true); err != nil {
			logger.Debug("failed to set multicast loopback", slog.Any("error", err))
		}
		for iface := range ifaces {
			if err := v4conn.JoinGroup(&ifaces[iface], mdnsGaddrUDP4); err != nil {
				logger.Debug("failed to join ipv4 multicast group", slog.String("interface", ifaces[iface].Name), slog.Any("error", err))
			} else {
				joined = true
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
		for iface := range ifaces {
			if err := v6conn.JoinGroup(&ifaces[iface], mdnsGaddrUDP6); err != nil {
				logger.Debug("failed to join ipv6 multicast group", slog.String("interface", ifaces[iface].Name), slog.Any("error", err))
			} else {
				joined = true
			}
		}
	}

	if !joined {
		logger.Warn("no multicast group joined; mDNS may not work")
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
	if s.recvCh != nil {
		s.wg.Wait()
		close(s.recvCh)
		s.recvCh = nil
	}
	return nil
}

func (s *sockets) send(buf []byte) error {
	var err4, err6 error

	// TODO: send multicast
	if s.conn4 != nil {
		_, err4 = s.conn4.WriteToUDP(buf, mdnsGaddrUDP4)
	}
	if s.conn6 != nil {
		_, err6 = s.conn6.WriteToUDP(buf, mdnsGaddrUDP6)
	}

	if err4 != nil && err6 != nil {
		return errors.Join(err4, err6)
	}
	if err4 != nil {
		logger.Warn("error sending DNS message via udp4", slog.Any("error", err4))
	}
	if err6 != nil {
		logger.Warn("error sending DNS message via udp6", slog.Any("error", err6))
	}

	return nil
}

func (s *sockets) receive(b []byte) (n int, err error) {
	var bufLock sync.Mutex

	s.recvOnce.Do(func() {
		s.recvCh = make(chan []byte)
		if s.conn4 != nil {
			s.wg.Go(func() {
				buf := make([]byte, len(b))
				for {
					n, _, err := s.conn4.ReadFromUDP(buf)
					if errors.Is(err, net.ErrClosed) {
						return
					}
					if err != nil {
						logger.Warn("error receiving DNS message via udp4", slog.Any("error", err))
						return
					}
					s.recvCh <- buf[:n]
					bufLock.Lock()
				}
			})
		}
		if s.conn6 != nil {
			s.wg.Go(func() {
				buf := make([]byte, len(b))
				for {
					n, _, err := s.conn6.ReadFromUDP(buf)
					if errors.Is(err, net.ErrClosed) {
						return
					}
					if err != nil {
						logger.Warn("error receiving DNS message via udp6", slog.Any("error", err))
						return
					}
					s.recvCh <- buf[:n]
					bufLock.Lock()
				}
			})
		}
	})

	data, ok := <-s.recvCh
	if !ok {
		return 0, net.ErrClosed
	}
	n = copy(b, data)
	bufLock.Unlock()
	return n, nil
}
