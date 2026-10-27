// Diagnostic SCVPN control-channel probe. Never installs routes or starts a tunnel.
package main

import (
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"time"
)

type field struct {
	kind uint16
	data []byte
}

func packet(kind uint16, fields ...field) []byte {
	b := make([]byte, 8)
	b[0] = 0x22
	binary.BigEndian.PutUint16(b[2:4], kind)
	for _, f := range fields {
		h := make([]byte, 4)
		binary.BigEndian.PutUint16(h, f.kind)
		binary.BigEndian.PutUint16(h[2:], uint16(len(f.data)))
		b = append(b, h...)
		b = append(b, f.data...)
		for len(b)%4 != 0 {
			b = append(b, 0)
		}
	}
	binary.BigEndian.PutUint32(b[4:8], uint32(len(b)))
	return b
}
func reply(r io.Reader, expected uint16) (uint32, error) {
	h := make([]byte, 8)
	if _, e := io.ReadFull(r, h); e != nil {
		return 0, e
	}
	size := binary.BigEndian.Uint32(h[4:])
	if h[0] != 0x22 || h[1] != 2 || binary.BigEndian.Uint16(h[2:4]) != expected || size < 8 || size > 65536 {
		return 0, errors.New("invalid SCVPN reply header")
	}
	body := make([]byte, int(size)-8)
	if _, e := io.ReadFull(r, body); e != nil {
		return 0, e
	}
	found := false
	var status uint32
	for len(body) > 0 {
		if len(body) < 4 {
			return 0, errors.New("truncated TLV header")
		}
		k, n := binary.BigEndian.Uint16(body[:2]), int(binary.BigEndian.Uint16(body[2:4]))
		padded := (n + 3) &^ 3
		if 4+padded > len(body) {
			return 0, errors.New("truncated TLV payload")
		}
		if k == 4 {
			if n != 4 || found {
				return 0, errors.New("invalid status field")
			}
			status = binary.BigEndian.Uint32(body[4:8])
			found = true
		}
		body = body[4+padded:]
	}
	if !found {
		return 0, errors.New("status field absent")
	}
	return status, nil
}
func uci(key string) (string, error) {
	b, e := exec.Command("uci", "-q", "get", "network.xdvpn."+key).Output()
	return strings.TrimSuffix(string(b), "\n"), e
}
func run() error {
	endpoint := flag.String("endpoint", "", "SCVPN host:port")
	pin := flag.String("sha256", "", "approved certificate SHA256; required")
	authenticate := flag.Bool("auth", false, "send saved router credentials; requires explicit certificate trust")
	flag.Parse()
	expected, e := hex.DecodeString(*pin)
	if e != nil || len(expected) != 32 {
		return errors.New("a 32-byte certificate SHA256 pin is required")
	}
	host, _, e := net.SplitHostPort(*endpoint)
	if e != nil {
		return errors.New("endpoint must be host:port")
	}
	conf := &tls.Config{MinVersion: tls.VersionTLS12, ServerName: host, InsecureSkipVerify: true,
		VerifyConnection: func(c tls.ConnectionState) error {
			if len(c.PeerCertificates) == 0 {
				return errors.New("missing certificate")
			}
			actual := sha256.Sum256(c.PeerCertificates[0].Raw)
			if !strings.EqualFold(hex.EncodeToString(actual[:]), *pin) {
				return errors.New("certificate pin mismatch")
			}
			return nil
		}}
	// InsecureSkipVerify replaces PKI checks ONLY with the mandatory exact leaf pin above.
	c, e := tls.DialWithDialer(&net.Dialer{Timeout: 8 * time.Second}, "tcp", *endpoint, conf)
	if e != nil {
		return e
	}
	defer c.Close()
	if e = c.SetDeadline(time.Now().Add(10 * time.Second)); e != nil {
		return e
	}
	var req []byte
	kind := uint16(2)
	if *authenticate {
		user, e := uci("username")
		if e != nil || user == "" {
			return errors.New("saved username missing")
		}
		password, e := uci("password")
		if e != nil || password == "" {
			return errors.New("saved password missing")
		}
		if len(user) > 1024 || len(password) > 1024 {
			return errors.New("credential too long")
		}
		kind = 1
		name, _ := os.Hostname()
		req = packet(kind, field{81, []byte{0, 1}}, field{1, []byte(user)}, field{2, []byte(password)}, field{84, []byte("1.0.0")}, field{96, nil}, field{97, []byte(name)})
	} else {
		ip := net.ParseIP(strings.Split(c.RemoteAddr().String(), ":")[0]).To4()
		if ip == nil {
			return errors.New("IPv4 endpoint required")
		}
		req = packet(kind, field{5, []byte{127, 0, 0, 1}}, field{6, ip})
	}
	if _, e = c.Write(req); e != nil {
		return e
	}
	clear(req)
	status, e := reply(c, kind)
	if e != nil {
		return e
	}
	fmt.Printf("SCVPN reply type=%d status=%d authenticated=%t\n", kind, status, *authenticate && status == 0)
	if *authenticate && status == 0 {
		_, e = c.Write(packet(7, field{83, []byte{0, 0}}))
		if e != nil {
			return e
		}
		fmt.Println("SCVPN logout sent; no routes or tunnel created")
	}
	return nil
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, "SCVPN probe:", e)
		os.Exit(1)
	}
}
