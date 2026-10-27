package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"
)

const ocState = "/var/run/xdvpn-openconnect.json"

type ocPolicy struct {
	Address, DNS, Endpoint, Mode string
	Protect                      bool
	Routes, Excludes             []string
	CompanyNetworks              []string
}

func (s ocPolicy) hook(reason, ips string) error {
	cmd := exec.Command("/usr/libexec/xdvpn/vpnc-script")
	cmd.Env = append(os.Environ(), "INTERFACE=xdvpn", "TUNDEV=vpn-xdvpn", "reason="+reason, "XDVPN_HELPER=1", "XDVPN_PRIORITY=1", "XDVPN_MODE="+s.Mode, "XDVPN_PROTECT="+fmt.Sprint(s.Protect), "INTERNAL_IP4_ADDRESS="+s.Address, "INTERNAL_IP4_DNS="+s.DNS, "XDVPN_ENDPOINT="+s.Endpoint, "XDVPN_COMPANY_READY=1", "XDVPN_COMPANY_NETWORKS="+strings.Join(s.CompanyNetworks, " "), "XDVPN_SERVER_EXCLUDES="+strings.Join(s.Excludes, " "), "XDVPN_DOMAIN_IPS="+ips)
	return cmd.Run()
}
func runOpenConnectPolicy(watch bool) error {
	var s ocPolicy
	if watch {
		b, e := os.ReadFile(ocState)
		if e != nil {
			return e
		}
		if e = json.Unmarshal(b, &s); e != nil {
			return e
		}
	} else {
		// Stop the preceding policy worker before applying reconnect data.
		exec.Command("/etc/init.d/xdvpn-policy", "stop").Run()
		s = ocPolicy{Address: os.Getenv("INTERNAL_IP4_ADDRESS"), DNS: os.Getenv("INTERNAL_IP4_DNS"), Endpoint: os.Getenv("VPNGATEWAY"), Mode: policyMode(uci("xdvpn_mode"))}
		s.Protect = protectEnabled(s.Mode, uci("xdvpn_protect_anthropic"))
		if net.ParseIP(s.Address).To4() == nil || net.ParseIP(s.Endpoint).To4() == nil {
			return errors.New("OpenConnect IPv4 address/endpoint missing")
		}
		var e error
		{
			s.Excludes, e = openConnectRoutes(os.Getenv, "CISCO_SPLIT_EXC", false)
			if e != nil {
				return e
			}
		}
	}
	s.Mode = policyMode(s.Mode)
	if s.Mode != "networks" && s.Mode != "company" && s.Mode != "global4" {
		return errors.New("invalid traffic mode")
	}
	raw, companyNetworks, e := policyTargets(s.Mode, uci("xdvpn_domains"))
	if e != nil {
		return e
	}
	s.CompanyNetworks = companyNetworks
	domains, e := newDomainRoutes(raw, net.ParseIP(s.Endpoint))
	if e != nil {
		return e
	}
	if !watch {
		if e := setupBlacklist(); e != nil {
			return e
		}
	}
	var suffixes []string
	if s.Protect {
		suffixes = proxyProtectedDomains()
		for _, name := range domains.names {
			if protectedDomain(name, suffixes) {
				return errors.New("protected domain conflicts with manual target")
			}
		}
		if !watch {
			if e = setupProtection(suffixes); e != nil {
				return e
			}
		}
	} else if !watch {
		if e = priority("disable-protection"); e != nil {
			return e
		}
	}
	if e := syncCompanyDomains(&domains, raw, net.ParseIP(s.Endpoint), suffixes, true); e != nil {
		return e
	}
	refresh := func() (string, error) {
		if s.Protect {
			if e := refreshProtection(suffixes); e != nil {
				return "", e
			}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
		defer cancel()
		ips, _ := domains.refresh(ctx, net.DefaultResolver.LookupIP, time.Now())
		return strings.Join(ips, " "), nil
	}
	_, e = refresh()
	if e != nil {
		return e
	}
	if !watch {
		if e = s.hook("connect", ""); e != nil {
			s.hook("disconnect", "")
			return e
		}
		b, _ := json.Marshal(s)
		if e = os.WriteFile(ocState, b, 0600); e != nil {
			s.hook("disconnect", "")
			return e
		}
		// Contains only route/session metadata, never credentials or VPN cookies.
		if e = exec.Command("/etc/init.d/xdvpn-policy", "start").Run(); e != nil {
			s.hook("disconnect", "")
			return e
		}
		return nil
	}
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGTERM, syscall.SIGINT)
	defer signal.Stop(sig)
	reload := make(chan os.Signal, 1)
	signal.Notify(reload, syscall.SIGUSR1)
	defer signal.Stop(reload)
	if e := os.WriteFile("/var/run/xdvpn-policy-ready", []byte(fmt.Sprint(os.Getpid())), 0600); e != nil {
		return e
	}
	defer os.Remove("/var/run/xdvpn-policy-ready")
	tick := time.NewTicker(5 * time.Minute)
	defer tick.Stop()
	steps := 0
	fail := func(err error) error { s.hook("disconnect", ""); return err }
	for {
		select {
		case <-sig:
			return nil
		case <-reload:
			if e := setupBlacklist(); e != nil {
				return fail(e)
			}
			s.Mode = policyMode(uci("xdvpn_mode"))
			if s.Mode != "networks" && s.Mode != "company" && s.Mode != "global4" {
				return fail(errors.New("invalid traffic mode"))
			}
			s.Protect = protectEnabled(s.Mode, uci("xdvpn_protect_anthropic"))
			suffixes = nil
			if s.Protect {
				suffixes = proxyProtectedDomains()
				if e := setupProtection(suffixes); e != nil {
					return fail(e)
				}
			} else if e := priority("disable-protection"); e != nil {
				return fail(e)
			}
			raw, nets, e := policyTargets(s.Mode, uci("xdvpn_domains"))
			if e != nil {
				return fail(e)
			}
			s.CompanyNetworks = nets
			if e := syncCompanyDomains(&domains, raw, net.ParseIP(s.Endpoint), suffixes, false); e != nil {
				return fail(e)
			}
			if _, e := refresh(); e != nil {
				return fail(e)
			}
			if e := s.hook("policy-update", ""); e != nil {
				return fail(e)
			}
			debug.FreeOSMemory()
			fmt.Println("OpenConnect policy refreshed without reconnect")
		case <-tick.C:
			if _, e = os.Stat("/sys/class/net/vpn-xdvpn"); e != nil {
				return nil
			}
			if e = priority("audit"); e != nil {
				return fail(e)
			}
			steps++
			if steps%2 == 0 {
				if _, e := refresh(); e != nil {
					return fail(e)
				}
			}
		}
	}
}
