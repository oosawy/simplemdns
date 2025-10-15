package transport

import (
	"errors"
	"net"
)

type Options struct {
	IPVersion      IPVersion
	BindTo         BindStrategy
	JoinIfaces     []net.Interface // nil or empty for all available multicast interfaces
	UDPRecvBufSize int             // should be in the range 1500-9000; smaller values may cause data loss
	MsgsChBufSize  int             // buffer size for the msgs channel; drops messages when full
}

func (o Options) withDefaults() (Options, error) {
	if len(o.JoinIfaces) == 0 {
		ifaces, err := multicastInterfaces()
		if err != nil {
			return Options{}, err
		}
		if len(ifaces) == 0 {
			return Options{}, errors.New("no multicast interfaces available")
		}
		o.JoinIfaces = ifaces
	}

	return o, nil
}
