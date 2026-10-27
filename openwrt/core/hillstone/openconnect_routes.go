package main

import (
	"errors"
	"fmt"
	"net"
	"strconv"
)

func openConnectRoutes(get func(string) string, prefix string, defaultRoute bool) ([]string, error) {
	count := get(prefix)
	if count == "" || count == "0" {
		if defaultRoute {
			return []string{"0.0.0.0/0"}, nil
		}
		return nil, nil
	}
	n, e := strconv.Atoi(count)
	if e != nil || n < 0 || n > 4096 {
		return nil, errors.New("invalid OpenConnect route count")
	}
	var routes []string
	for i := 0; i < n; i++ {
		key := fmt.Sprintf("%s_%d_", prefix, i)
		ip := net.ParseIP(get(key + "ADDR")).To4()
		if ip == nil {
			return nil, errors.New("invalid OpenConnect route address")
		}
		bits, e := strconv.Atoi(get(key + "MASKLEN"))
		if e != nil {
			mask := net.ParseIP(get(key + "MASK")).To4()
			if mask == nil {
				return nil, errors.New("missing route mask")
			}
			var size int
			bits, size = net.IPMask(mask).Size()
			if size != 32 {
				return nil, errors.New("invalid route mask")
			}
		}
		if bits < 0 || bits > 32 {
			return nil, errors.New("invalid route prefix")
		}
		network := ip.Mask(net.CIDRMask(bits, 32))
		routes = append(routes, fmt.Sprintf("%s/%d", network.String(), bits))
	}
	return routes, nil
}
