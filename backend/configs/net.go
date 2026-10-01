package configs

import (
	"fmt"
	"net"
	"strings"
)

// ParseNet reads an address or a range: "203.0.113.7" or "203.0.113.0/24".
func ParseNet(v string) (*net.IPNet, error) {
	v = strings.TrimSpace(v)
	if strings.Contains(v, "/") {
		_, n, err := net.ParseCIDR(v)
		if err != nil {
			return nil, fmt.Errorf("%q geçerli bir adres aralığı değil", v)
		}
		return n, nil
	}
	ip := net.ParseIP(v)
	if ip == nil {
		return nil, fmt.Errorf("%q geçerli bir IP adresi değil", v)
	}
	bits := 32
	if ip.To4() == nil {
		bits = 128
	}
	return &net.IPNet{IP: ip, Mask: net.CIDRMask(bits, bits)}, nil
}

// Nets reads a comma separated list of addresses and ranges, skipping the
// ones that cannot be read (Check reports those).
func Nets(list string) []*net.IPNet {
	var out []*net.IPNet
	for _, v := range splitList(list) {
		if n, err := ParseNet(v); err == nil {
			out = append(out, n)
		}
	}
	return out
}

// Contains reports whether ip falls in any of nets.
func Contains(nets []*net.IPNet, ip string) bool {
	addr := net.ParseIP(strings.TrimSpace(ip))
	if addr == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(addr) {
			return true
		}
	}
	return false
}
