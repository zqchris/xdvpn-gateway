// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"crypto/rand"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

func uci(key string) string {
	b, _ := exec.Command("uci", "-q", "get", "network.xdvpn."+key).Output()
	return strings.TrimSuffix(string(b), "\n")
}

var managed *bool
var testRoute *string

type permanentError struct{ error }

func permanent(e error) error { return &permanentError{e} }
func run() error {
	endpoint := flag.String("endpoint", "", "host:port")
	pin := flag.String("sha256", "", "trusted certificate hash")
	duration := flag.Duration("duration", 0, "bounded tunnel test duration; zero negotiates only")
	managed = flag.Bool("netifd", false, "integrate with the owned OpenWrt interface")
	testRoute = flag.String("test-route", "", "single IPv4 CIDR for a bounded diagnostic")
	info := flag.Bool("certificate-info", false, "inspect server certificate without sending VPN credentials")
	policyEvent := flag.Bool("policy-openconnect", false, "prepare the shared policy for an OpenConnect event")
	policyWatch := flag.Bool("policy-watch", false, "maintain OpenConnect domain and priority rules")
	activity := flag.Bool("activity-info", false, "read filtered activity for LuCI")
	domain := flag.Bool("domain-info", false, "inspect current domain policy for LuCI")
	company := flag.Bool("company-info", false, "read private company rule counts")
	importRules := flag.Bool("company-import", false, "import private rules from RPC stdin")
	flag.Parse()
	if *importRules {
		return importCompanyRules()
	}
	if *company {
		return companyInfo()
	}
	if *activity {
		return activityInfo()
	}
	if *domain {
		return domainInfo()
	}
	if *policyEvent || *policyWatch {
		return runOpenConnectPolicy(*policyWatch)
	}
	if *endpoint == "" {
		*endpoint = net.JoinHostPort(uci("server"), uci("port"))
	}
	if *info {
		return certificateInfo(*endpoint)
	}
	if *pin == "" {
		*pin = uci("xdvpn_certpin")
	}
	p, e := hex.DecodeString(*pin)
	if *pin != "" && (e != nil || len(p) != 32) {
		return permanent(errors.New("valid certificate pin required"))
	}
	host, _, e := net.SplitHostPort(*endpoint)
	if e != nil {
		return e
	}
	if *pin != "" && uci("xdvpn_cert_endpoint") != *endpoint {
		return permanent(errors.New("saved certificate trust belongs to another endpoint; review certificate"))
	}
	conf := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, InsecureSkipVerify: true, VerifyConnection: func(s tls.ConnectionState) error {
		return verifyServer(s, host, *pin)
	}}
	c, e := tls.DialWithDialer(&net.Dialer{Timeout: 8 * time.Second}, "tcp4", *endpoint, conf)
	if e != nil {
		return e
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(25 * time.Second))
	user, pass := uci("username"), uci("password")
	if user == "" || pass == "" || len(user) > 1024 || len(pass) > 1024 {
		return permanent(errors.New("invalid credentials configuration"))
	}
	name, _ := os.Hostname()
	if e = send(c, 1, num(81, 1, 2), field{1, []byte(user)}, field{2, []byte(pass)}, field{84, []byte("1.0.0")}, field{96, nil}, field{97, []byte(name)}); e != nil {
		return e
	}
	if _, e = expect(c, 1); e != nil {
		var rejected *statusError
		if errors.As(e, &rejected) {
			return permanent(e)
		}
		return e
	}
	fmt.Println("SCVPN authenticated")
	defer func() { c.SetWriteDeadline(time.Now().Add(time.Second)); send(c, 7, num(83, 0, 2)) }()
	remote := c.RemoteAddr().(*net.TCPAddr).IP.To4()
	local := c.LocalAddr().(*net.TCPAddr).IP.To4()
	if e = send(c, 2, field{5, local}, field{6, remote}); e != nil {
		return e
	}
	if _, e = expect(c, 2); e != nil {
		return e
	}
	var config message
	var routeData []byte
	for i := 0; i < 32; i++ {
		m, e := readMessage(c)
		if e != nil {
			return e
		}
		if e = m.status(); e != nil {
			return e
		}
		fmt.Printf("SCVPN config type=%d fields=%d\n", m.kind, len(m.fields))
		if m.kind == 3 {
			config = m
		}
		if m.kind == 4 {
			routeData = append(routeData, m.fields[19]...)
		}
		if m.kind == 6 {
			break
		}
		if i == 31 {
			return errors.New("too many negotiation messages")
		}
	}
	if config.fields != nil {
		config.fields[19] = routeData
	}
	port, e := config.number(9)
	if e != nil || port == 0 || port > 65535 {
		return errors.New("invalid UDP port")
	}
	key := make([]byte, 48)
	if _, e = rand.Read(key); e != nil {
		return e
	}
	defer clear(key)
	spi := make([]byte, 4)
	if _, e = rand.Read(spi); e != nil {
		return e
	}
	if e = send(c, 5, num(32, 3, 2), field{16, key}, field{48, spi}, num(128, 0, 2)); e != nil {
		return e
	}
	k, e := expect(c, 5)
	if e != nil {
		return e
	}
	enc, e := k.number(49)
	if e != nil {
		return e
	}
	auth, e := k.number(50)
	if e != nil {
		return e
	}
	comp, e := k.number(129)
	if e != nil {
		return e
	}
	if comp != 0 {
		return permanent(errors.New("server requires unsupported compression"))
	}
	fmt.Printf("SCVPN negotiated enc=%d auth=%d compression=%d udp_port=%d assigned_ipv4=%t\n", enc, auth, comp, port, len(config.fields[7]) == 4)
	if *duration > 0 || *managed {
		return dataTunnel(c, config, k, key, spi, remote, port, enc, auth, *duration)
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "hillstone:", e)
		var p *permanentError
		if errors.As(e, &p) {
			os.Exit(2)
		}
		os.Exit(1)
	}
}
