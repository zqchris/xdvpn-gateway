package main

import (
	"errors"
	"net"
	"sort"
	"strings"
)

func parseBlacklist(raw string) (domains, networks []string, err error) {
	entries := strings.Fields(raw)
	if len(entries) > 256 {
		return nil, nil, errors.New("at most 256 blacklist entries")
	}
	seen := map[string]bool{}
	for _, v := range entries {
		v = strings.ToLower(strings.TrimSuffix(v, "."))
		if seen[v] {
			continue
		}
		seen[v] = true
		if ip := net.ParseIP(v); ip != nil && ip.To4() != nil {
			networks = append(networks, ip.String()+"/32")
			continue
		}
		if _, n, e := net.ParseCIDR(v); e == nil && n.IP.To4() != nil {
			ones, _ := n.Mask.Size()
			if ones == 0 {
				return nil, nil, errors.New("blacklist /0 not supported")
			}
			networks = append(networks, n.String())
			continue
		}
		if !validDomain(v) || strings.Trim(v, "0123456789.") == "" {
			return nil, nil, errors.New("invalid blacklist domain or IPv4 network")
		}
		domains = append(domains, v)
	}
	sort.Strings(domains)
	sort.Strings(networks)
	return
}
