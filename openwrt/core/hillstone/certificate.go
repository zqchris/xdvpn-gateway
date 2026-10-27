// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"strings"
	"time"
)

func certificateHash(cert *x509.Certificate) string {
	h := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(h[:])
}
func verifyServer(s tls.ConnectionState, host, pin string) error {
	if len(s.PeerCertificates) == 0 {
		return errors.New("missing server certificate")
	}
	if pin != "" {
		if !strings.EqualFold(certificateHash(s.PeerCertificates[0]), pin) {
			return permanent(errors.New("server certificate changed; confirmation required"))
		}
		return nil
	}
	pool := x509.NewCertPool()
	for _, cert := range s.PeerCertificates[1:] {
		pool.AddCert(cert)
	}
	_, err := s.PeerCertificates[0].Verify(x509.VerifyOptions{DNSName: host, Intermediates: pool})
	if err != nil {
		return permanent(errors.New("server certificate not trusted; review certificate in XDVPN before connecting"))
	}
	return nil
}

// Fetch public TLS identity only. Never reads or sends VPN credentials.
func certificateInfo(endpoint string) error {
	host, _, err := net.SplitHostPort(endpoint)
	if err != nil {
		return err
	}
	c, err := tls.DialWithDialer(&net.Dialer{Timeout: 8 * time.Second}, "tcp4", endpoint, &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, InsecureSkipVerify: true})
	if err != nil {
		return err
	}
	defer c.Close()
	s := c.ConnectionState()
	if len(s.PeerCertificates) == 0 {
		return errors.New("missing certificate")
	}
	cert := s.PeerCertificates[0]
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"endpoint": endpoint, "sha256": certificateHash(cert), "subject": cert.Subject.String(), "issuer": cert.Issuer.String(),
		"expires": cert.NotAfter.UTC().Format(time.RFC3339), "ca_valid": verifyServer(s, host, "") == nil,
		"saved_pin_matches": strings.EqualFold(uci("xdvpn_certpin"), certificateHash(cert)),
	})
}
