// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"time"
)

type domainEntry struct {
	ips     []string
	updated time.Time
}
type domainRoutes struct {
	names    []string
	entries  map[string]domainEntry
	endpoint net.IP
}

func validDomain(s string) bool {
	if len(s) > 253 || !strings.Contains(s, ".") || net.ParseIP(s) != nil {
		return false
	}
	for _, label := range strings.Split(s, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, c := range label {
			if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
				return false
			}
		}
	}
	return true
}
func newDomainRoutes(raw string, endpoint net.IP) (*domainRoutes, error) {
	d := &domainRoutes{entries: make(map[string]domainEntry), endpoint: endpoint}
	seen := make(map[string]bool)
	for _, s := range strings.Fields(raw) {
		s = strings.ToLower(strings.TrimSuffix(s, "."))
		if !validDomain(s) {
			return nil, errors.New("invalid company domain; use an exact hostname")
		}
		if !seen[s] {
			d.names = append(d.names, s)
			seen[s] = true
		}
	}
	if len(d.names) > 320 {
		return nil, errors.New("at most 320 combined company domains supported")
	}
	return d, nil
}
func (d *domainRoutes) refresh(ctx context.Context, lookup func(context.Context, string, string) ([]net.IP, error), now time.Time) ([]string, int) {
	failures := 0
	for _, name := range d.names {
		ips, err := lookup(ctx, "ip4", name)
		if err != nil {
			failures++
			continue
		}
		entry := domainEntry{updated: now}
		for _, ip := range ips {
			if ip.To4() == nil || !ip.IsGlobalUnicast() || ip.Equal(d.endpoint) {
				continue
			}
			entry.ips = append(entry.ips, ip.To4().String())
			if len(entry.ips) >= 32 {
				break
			}
		}
		d.entries[name] = entry
	}
	seen := make(map[string]bool)
	for _, entry := range d.entries {
		// Transient DNS errors retain the previous result for at most ten minutes.
		if now.Sub(entry.updated) > 10*time.Minute {
			continue
		}
		for _, ip := range entry.ips {
			seen[ip] = true
		}
	}
	result := make([]string, 0, len(seen))
	for ip := range seen {
		result = append(result, ip)
	}
	sort.Strings(result)
	if len(result) > 512 {
		result = result[:512]
	}
	return result, failures
}
