package main

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Own DNS snippet and sets; always excluded, even in global mode. No proxy edits.
func setupBlacklist() error {
	raw := uci("xdvpn_blacklist")
	domains, networks, e := parseBlacklist(raw)
	if e != nil {
		return e
	}
	if e = priority("init"); e != nil {
		return e
	}
	var lines []string
	for _, d := range domains {
		lines = append(lines, "nftset=/"+d+"/4#inet#xdvpn_guard#block4,4#inet#fw4#xdvpn_block4")
	}
	if e = writeDNSRules("92-xdvpn-blacklist.conf", []byte(strings.Join(lines, "\n")+"\n")); e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	for _, d := range domains {
		ips, e := net.DefaultResolver.LookupIP(ctx, "ip4", d)
		if e != nil {
			continue
		}
		for _, ip := range ips {
			if ip.To4() != nil {
				networks = append(networks, ip.String()+"/32")
			}
		}
	}
	// Keep child DNS discoveries on ordinary refresh; clear when the user removes
	// entries so deleted exclusions do not remain indefinitely.
	previous, _ := os.ReadFile("/var/run/xdvpn-blacklist-config")
	retained := map[string]bool{}
	for _, v := range strings.Fields(raw) {
		retained[v] = true
	}
	removed := false
	for _, v := range strings.Fields(string(previous)) {
		if !retained[v] {
			removed = true
		}
	}
	var batch strings.Builder
	for _, set := range []struct{ table, name string }{{"xdvpn_guard", "block4"}, {"fw4", "xdvpn_block4"}} {
		if removed {
			fmt.Fprintf(&batch, "flush set inet %s %s\n", set.table, set.name)
		}
		if len(networks) > 0 {
			fmt.Fprintf(&batch, "add element inet %s %s { %s }\n", set.table, set.name, strings.Join(networks, ", "))
		}
	}
	if batch.Len() > 0 {
		cmd := exec.Command("nft", "-f", "-")
		cmd.Stdin = strings.NewReader(batch.String())
		if e = cmd.Run(); e != nil {
			return errors.New("cannot apply custom blacklist")
		}
	}
	return os.WriteFile("/var/run/xdvpn-blacklist-config", []byte(raw), 0600)
}
