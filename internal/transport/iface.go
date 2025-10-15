package transport

import "net"

func multicastInterfaces() ([]net.Interface, error) {
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

func interfaceSupports(iface *net.Interface, v IPVersion) (supports bool, err error) {
	addrs, err := iface.Addrs()
	if err != nil {
		return false, err
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

		if v == IPv4 && ip.To4() != nil {
			return true, nil
		}
		if v == IPv6 && ip.To16() != nil && ip.To4() == nil {
			return true, nil
		}
	}

	return
}
