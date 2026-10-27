// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

func proxyProtectedDomains() []string {
	seen := make(map[string]bool)
	for _, s := range protectedSuffixes {
		seen[s] = true
	}
	var port string
	get := func(key string) string {
		b, _ := exec.Command("uci", "-q", "get", "openclash.config."+key).Output()
		return strings.TrimSpace(string(b))
	}
	port = get("cn_port")
	if port == "" {
		port = "9090"
	}
	req, err := http.NewRequest("GET", "http://127.0.0.1:"+port+"/rules", nil)
	if err == nil {
		req.Header.Set("Authorization", "Bearer "+get("dashboard_password"))
		client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{Proxy: nil}}
		if res, e := client.Do(req); e == nil {
			var data struct {
				Rules []struct{ Type, Payload, Proxy string }
			}
			if res.StatusCode == 200 && json.NewDecoder(io.LimitReader(res.Body, 4<<20)).Decode(&data) == nil {
				for _, r := range data.Rules {
					group := strings.ToLower(r.Proxy)
					if (strings.Contains(group, "claude") || strings.Contains(group, "anthropic")) && (r.Type == "DomainSuffix" || r.Type == "Domain") && validDomain(r.Payload) {
						seen[strings.ToLower(r.Payload)] = true
					}
				}
			}
			res.Body.Close()
		}
		client.CloseIdleConnections()
	}
	result := make([]string, 0, len(seen))
	for s := range seen {
		result = append(result, s)
	}
	sort.Strings(result)
	return result
}
func priority(action string) error { return exec.Command("/usr/libexec/xdvpn/priority", action).Run() }
func setupProtection(suffixes []string) error {
	version, err := exec.Command("dnsmasq", "--version").Output()
	if err != nil || !strings.Contains(string(version), " nftset ") {
		return errors.New("Anthropic protection requires dnsmasq with nftset support")
	}
	if err := priority("init"); err != nil {
		return errors.New("VPN protection requires working nftables/fw4")
	}
	var lines []string
	for _, s := range suffixes {
		lines = append(lines, "nftset=/"+s+"/4#inet#xdvpn_guard#deny4,4#inet#fw4#xdvpn_deny4,6#inet#xdvpn_guard#deny6")
	}
	if err := writeDNSRules("90-xdvpn-protect.conf", []byte(strings.Join(lines, "\n")+"\n")); err != nil {
		return err
	}
	var readyErr error
	for attempt := 0; attempt < 10; attempt++ {
		readyErr = refreshProtection(suffixes)
		if readyErr == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return readyErr
}
func refreshProtection(suffixes []string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	names := append(append([]string{}, suffixes...), "api.anthropic.com", "console.anthropic.com", "platform.claude.com", "www.anthropic.com", "www.claude.com", "www.claudeusercontent.com")
	seen := make(map[string]bool)
	for _, name := range names {
		ips, e := net.DefaultResolver.LookupIP(ctx, "ip4", name)
		if e != nil {
			continue
		}
		for _, ip := range ips {
			if ip.To4() != nil && ip.IsGlobalUnicast() {
				seen[ip.To4().String()] = true
			}
		}
	}
	if len(seen) == 0 {
		return errors.New("cannot resolve protected domains; refusing VPN connection")
	}
	var ips []string
	for ip := range seen {
		ips = append(ips, ip)
	}
	sort.Strings(ips)
	// Never flush protected discoveries: shared-IP conflicts fail closed.
	batch := fmt.Sprintf("add element inet xdvpn_guard deny4 { %s }\nadd element inet fw4 xdvpn_deny4 { %s }\n", strings.Join(ips, ", "), strings.Join(ips, ", "))
	cmd := exec.Command("nft", "-f", "-")
	cmd.Stdin = strings.NewReader(batch)
	if err := cmd.Run(); err != nil {
		return errors.New("cannot populate protected destination sets")
	}
	return nil
}

func writeDNSRules(filename string, content []byte) error {
	files, _ := filepath.Glob("/tmp/etc/dnsmasq.conf.*")
	dirs := make(map[string]bool)
	for _, p := range files {
		b, e := os.ReadFile(p)
		if e != nil {
			continue
		}
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "conf-dir=") {
				dir := strings.Split(strings.TrimPrefix(line, "conf-dir="), ",")[0]
				if filepath.IsAbs(dir) {
					dirs[dir] = true
				}
			}
		}
	}
	if len(dirs) == 0 {
		return errors.New("cannot find dnsmasq configuration directory; protection unavailable")
	}
	changed := false
	for dir := range dirs {
		path := filepath.Join(dir, filename)
		old, _ := os.ReadFile(path)
		if string(old) == string(content) {
			continue
		}
		if err := os.WriteFile(path+".new", content, 0600); err != nil {
			return err
		}
		if err := os.Rename(path+".new", path); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		if err := exec.Command("/etc/init.d/dnsmasq", "restart").Run(); err != nil {
			return errors.New("DNS protection reload failed")
		}
		ready := false
		for attempt := 0; attempt < 20; attempt++ {
			conn, err := net.DialTimeout("tcp4", "127.0.0.1:53", 250*time.Millisecond)
			if err == nil {
				conn.Close()
				ready = true
				break
			}
			time.Sleep(250 * time.Millisecond)
		}
		if !ready {
			return errors.New("DNS did not become ready after reload")
		}
	}
	return nil
}

func setupCompanyDomains(names []string) error {
	if err := priority("init"); err != nil {
		return err
	}
	version, err := exec.Command("dnsmasq", "--version").Output()
	if err != nil || !strings.Contains(string(version), " nftset ") {
		return errors.New("company suffix rules require dnsmasq nftset")
	}
	var lines []string
	for _, name := range names {
		if !validDomain(name) {
			return errors.New("invalid company suffix")
		}
		lines = append(lines, "nftset=/"+name+"/4#inet#xdvpn_guard#domain4,4#inet#fw4#xdvpn_domain4")
	}
	return writeDNSRules("91-xdvpn-company.conf", []byte(strings.Join(lines, "\n")+"\n"))
}

// Refresh edited domain lists without requiring a transport reconnect.
func syncCompanyDomains(d **domainRoutes, raw string, endpoint net.IP, suffixes []string, first bool) error {
	next, err := newDomainRoutes(raw, endpoint)
	if err != nil {
		return err
	}
	for _, name := range next.names {
		if protectedDomain(name, suffixes) {
			return errors.New("protected domain conflicts with company target")
		}
	}
	if !first && strings.Join((*d).names, " ") == strings.Join(next.names, " ") {
		return nil
	}
	removed := false
	retained := make(map[string]bool)
	for _, name := range next.names {
		retained[name] = true
	}
	for _, name := range (*d).names {
		if !retained[name] {
			removed = true
		}
	}
	// Drop discoveries for removed suffixes before installing the new DNS rules.
	for _, set := range []struct{ table, name string }{{"xdvpn_guard", "domain4"}, {"fw4", "xdvpn_domain4"}} {
		if !first && removed {
			if err := exec.Command("nft", "flush", "set", "inet", set.table, set.name).Run(); err != nil {
				return err
			}
		}
	}
	if err := setupCompanyDomains(next.names); err != nil {
		return err
	}
	*d = next
	return nil
}
