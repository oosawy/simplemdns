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

func interfaceIPVersion(iface *net.Interface) (hasIPv4, hasIPv6 bool, err error) {
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
