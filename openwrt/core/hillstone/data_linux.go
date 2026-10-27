// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

func link(args ...string) error { return exec.Command("ip", args...).Run() }
func dataTunnel(c *tls.Conn, config, k message, key, spi []byte, remote net.IP, port, enc, auth uint32, duration time.Duration) error {
	mode := policyMode(uci("xdvpn_mode"))
	protect := protectEnabled(mode, uci("xdvpn_protect_anthropic"))
	var suffixes []string
	if *managed {
		if e := setupBlacklist(); e != nil {
			return permanent(e)
		}
		if mode != "networks" && mode != "company" && mode != "global4" {
			return permanent(errors.New("invalid traffic mode"))
		}
		if protect {
			suffixes = proxyProtectedDomains()
			if mode != "global4" {
				for _, name := range strings.Fields(uci("xdvpn_domains")) {
					if protectedDomain(name, suffixes) {
						return permanent(errors.New("protected domain conflicts with manual company target"))
					}
				}
			}
			if e := setupProtection(suffixes); e != nil {
				return permanent(e)
			}
		} else if e := priority("disable-protection"); e != nil {
			return permanent(e)
		}
		debug.FreeOSMemory()
	}
	assigned := net.IP(config.fields[7])
	if len(assigned) != 4 {
		return errors.New("invalid assigned IP")
	}
	outSPI, e := k.number(48)
	if e != nil {
		return e
	}
	in, out, e := associations(key, binary.BigEndian.Uint32(spi), outSPI, enc, auth)
	if e != nil {
		return permanent(e)
	}
	udp, e := net.DialUDP("udp4", nil, &net.UDPAddr{IP: remote, Port: int(port)})
	if e != nil {
		return e
	}
	defer udp.Close()
	f, e := openTUN("vpn-xdvpn")
	if e != nil {
		return e
	}
	defer f.Close()
	name := "vpn-xdvpn"
	if e = link("link", "set", "dev", name, "mtu", "1380", "up"); e != nil {
		return e
	}
	dnsRaw := config.fields[17]
	if len(dnsRaw)%4 != 0 {
		return errors.New("invalid DNS list")
	}
	var dnsList []string
	for off := 0; off < len(dnsRaw); off += 4 {
		dnsList = append(dnsList, net.IP(dnsRaw[off:off+4]).String())
	}
	domainConfig, companyNetworks, e := policyTargets(mode, uci("xdvpn_domains"))
	if e != nil {
		return permanent(e)
	}
	domains, e := newDomainRoutes(domainConfig, remote)
	if e != nil {
		return permanent(e)
	}
	if !*managed || mode == "global4" {
		domains.names = nil
	}
	if *managed {
		if e := syncCompanyDomains(&domains, domainConfig, remote, suffixes, true); e != nil {
			return permanent(e)
		}
	}
	refreshDomains := func() string {
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		ips, failures := domains.refresh(ctx, net.DefaultResolver.LookupIP, time.Now())
		if len(domains.names) > 0 {
			fmt.Printf("SCVPN domains configured=%d resolved_ips=%d unresolved=%d\n", len(domains.names), len(ips), failures)
		}
		return strings.Join(ips, " ")
	}
	refreshDomains()
	hook := func(reason string) error {
		cmd := exec.Command("/usr/libexec/xdvpn/vpnc-script")
		cmd.Env = append(os.Environ(), "INTERFACE=xdvpn", "TUNDEV="+name, "reason="+reason, "INTERNAL_IP4_ADDRESS="+assigned.String(), "INTERNAL_IP4_DNS="+strings.Join(dnsList, " "), "XDVPN_DOMAIN_IPS=", "XDVPN_PRIORITY=1", "XDVPN_MODE="+mode, "XDVPN_PROTECT="+fmt.Sprint(protect), "XDVPN_COMPANY_READY=1", "XDVPN_COMPANY_NETWORKS="+strings.Join(companyNetworks, " "), "XDVPN_ENDPOINT="+remote.String())
		return cmd.Run()
	}
	if *managed {
		defer hook("disconnect")
		if e = hook("connect"); e != nil {
			return fmt.Errorf("netifd setup: %w", e)
		}
	} else {
		if e = link("addr", "add", assigned.String()+"/32", "dev", name); e != nil {
			return e
		}
		target, n, e := net.ParseCIDR(*testRoute)
		if e != nil || target.To4() == nil {
			return errors.New("IPv4 test-route required")
		}
		if e = link("route", "add", n.String(), "dev", name); e != nil {
			return e
		}
		for _, dns := range dnsList {
			if e = link("route", "add", dns+"/32", "dev", name); e != nil {
				return e
			}
		}
	}
	c.SetDeadline(time.Time{})
	access := newAccessRecorder("")
	access.zone = routerLocation()
	stopAccess, accessErr := access.serve()
	if accessErr != nil {
		fmt.Println("SCVPN access log unavailable")
	}
	defer stopAccess()
	var tx, rx, drop atomic.Uint64
	errch := make(chan error, 3)
	go func() {
		b := make([]byte, 65536)
		dst := make([]byte, 65536)
		for {
			n, e := f.Read(b)
			if e != nil {
				errch <- e
				return
			}
			if n < 20 || b[0]>>4 != 4 {
				continue
			}
			p, e := out.encrypt(dst, b[:n])
			if e != nil {
				errch <- e
				return
			}
			if _, e = udp.Write(p); e != nil {
				errch <- e
				return
			}
			tx.Add(1)
			access.observe(b[:n], time.Now())
		}
	}()
	go func() {
		b := make([]byte, 65536)
		for {
			n, e := udp.Read(b)
			if e != nil {
				errch <- e
				return
			}
			p, e := in.decrypt(b[:n])
			if e != nil {
				drop.Add(1)
				continue
			}
			if _, e = f.Write(p); e != nil {
				errch <- e
				return
			}
			rx.Add(1)
		}
	}()
	go func() {
		for {
			m, e := readMessage(c)
			if e == io.EOF {
				fmt.Println("SCVPN control channel closed; UDP liveness monitored separately")
				return
			}
			if e != nil {
				errch <- e
				return
			}
			fmt.Printf("SCVPN control type=%d reply=%t fields=%d\n", m.kind, m.reply, len(m.fields))
			if m.kind == 8 {
				errch <- errors.New("server disconnect")
				return
			}
			if m.kind == 10 {
				errch <- errors.New("server rekey requested; prototype stops safely")
				return
			}
		}
	}()
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGUSR1)
	defer signal.Stop(reload)
	if e := os.WriteFile("/var/run/xdvpn-policy-ready", []byte(fmt.Sprint(os.Getpid())), 0600); e != nil {
		return e
	}
	defer os.Remove("/var/run/xdvpn-policy-ready")
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sig)
	tick := time.NewTicker(30 * time.Second)
	defer tick.Stop()
	refreshPolicy := func() error {
		if e := setupBlacklist(); e != nil {
			return e
		}
		nextMode := policyMode(uci("xdvpn_mode"))
		if nextMode != "networks" && nextMode != "company" && nextMode != "global4" {
			return errors.New("invalid traffic mode")
		}
		mode = nextMode

		protect = protectEnabled(mode, uci("xdvpn_protect_anthropic"))
		suffixes = nil
		if protect {
			suffixes = proxyProtectedDomains()
			if e := setupProtection(suffixes); e != nil {
				return e
			}
		} else if e := priority("disable-protection"); e != nil {
			return e
		}
		raw, networks, e := policyTargets(mode, uci("xdvpn_domains"))
		if e != nil {
			return e
		}
		companyNetworks = networks
		if e := syncCompanyDomains(&domains, raw, remote, suffixes, false); e != nil {
			return e
		}
		refreshDomains()
		if e := hook("policy-update"); e != nil {
			return e
		}
		debug.FreeOSMemory()
		fmt.Println("SCVPN policy refreshed without reconnect")
		return nil
	}
	var expiry <-chan time.Time
	if duration > 0 {
		limit := time.NewTimer(duration)
		defer limit.Stop()
		expiry = limit.C
	}
	start := time.Now()
	misses := 0
	samples := 0
	lastRX := rx.Load()
	fmt.Println("SCVPN tunnel ready")
	for {
		select {
		case <-reload:
			if e := refreshPolicy(); e != nil {
				return fmt.Errorf("policy refresh: %w", e)
			}
		case e := <-errch:
			return e
		case <-sig:
			return nil
		case <-expiry:
			return nil
		case <-tick.C:
			samples++
			if *managed && samples%10 == 0 {
				if e := priority("audit"); e != nil {
					return errors.New("VPN policy audit failed")
				}
			}
			if samples%20 == 0 {
				if protect {
					if e := refreshProtection(suffixes); e != nil {
						return e
					}
				}
				refreshDomains()
			}
			currentRX := rx.Load()
			if currentRX != lastRX {
				misses = 0
			} else if len(dnsList) > 0 {
				if probeDNS(assigned.String(), dnsList[0], name) {
					misses = 0
				} else {
					misses++
					fmt.Printf("SCVPN health missed=%d/3\n", misses)
				}
				if misses >= 3 {
					return errors.New("data channel unresponsive")
				}
			}
			lastRX = rx.Load()
			if samples%10 == 0 {
				fmt.Printf("SCVPN stats elapsed=%ds tx=%d rx=%d drops=%d\n", int(time.Since(start).Seconds()), tx.Load(), rx.Load(), drop.Load())
			}

		}
	}
}

func probeDNS(source, target, device string) bool {
	d := net.Dialer{Timeout: 3 * time.Second, LocalAddr: &net.UDPAddr{IP: net.ParseIP(source)}, Control: func(network, address string, c syscall.RawConn) error {
		var err error
		e := c.Control(func(fd uintptr) {
			err = syscall.SetsockoptString(int(fd), syscall.SOL_SOCKET, syscall.SO_BINDTODEVICE, device)
		})
		if e != nil {
			return e
		}
		return err
	}}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	c, e := d.DialContext(ctx, "udp4", net.JoinHostPort(target, "53"))
	if e != nil {
		return false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(3 * time.Second))
	q := []byte{0, 0, 1, 0, 0, 1, 0, 0, 0, 0, 0, 0, 0, 0, 2, 0, 1}
	if _, e = rand.Read(q[:2]); e != nil {
		return false
	}
	if _, e = c.Write(q); e != nil {
		return false
	}
	var b [512]byte
	n, e := c.Read(b[:])
	return e == nil && n >= 12 && b[0] == q[0] && b[1] == q[1] && b[2]&0x80 != 0
}
