// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
)

// SCVPN ROUTE_IPV4 is an array of IPv4 address, netmask, metric (3 x BE32).
func serverRoutes(raw []byte) ([]string, error) {
	if len(raw)%12 != 0 || len(raw) > 12*4096 {
		return nil, errors.New("invalid server IPv4 routes")
	}
	var result []string
	for i := 0; i < len(raw); i += 12 {
		mask := net.IPMask(raw[i+4 : i+8])
		ones, bits := mask.Size()
		if bits != 32 {
			return nil, errors.New("non-contiguous server route mask")
		}
		ip := net.IP(raw[i : i+4])
		network := ip.Mask(mask)
		if !ip.Equal(network) {
			return nil, errors.New("unaligned server route")
		}
		_ = binary.BigEndian.Uint32(raw[i+8 : i+12]) // Metric is relative to server routes, not proxy priority.
		result = append(result, fmt.Sprintf("%s/%d", network.String(), ones))
	}
	return result, nil
}
func protectEnabled(mode, value string) bool { return mode != "global4" && value != "0" }
