package main

// Access metadata is observed only after a packet is handed to the VPN transport.
// No packet capture process, payload persistence, reverse-DNS guesses or disk log.
import (
	"context"
	"encoding/binary"
	"encoding/json"
	"io"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

const accessSocket = "/var/run/xdvpn-access.sock"

type accessRow struct {
	Outer       bool     `json:"outer,omitempty"`
	Group       string   `json:"group"`
	Domains     []string `json:"domains"`
	IPs         []string `json:"ips"`
	First       string   `json:"first"`
	Last        string   `json:"last"`
	Connections int      `json:"connections"`
	seen        time.Time
}
type accessFlow struct {
	outer bool
	host  string
	seq   uint32
	data  []byte
	done  bool
	seen  time.Time
}
type accessRecorder struct {
	zone     *time.Location
	mu       sync.Mutex
	rows     []accessRow
	buffered int
	flows    map[[13]byte]*accessFlow // owned by the single TUN transmit goroutine
}

func newAccessRecorder(parents string) *accessRecorder {
	return &accessRecorder{flows: make(map[[13]byte]*accessFlow)}
}
func boundedAppend(xs []string, s string) []string {
	for _, x := range xs {
		if x == s {
			return xs
		}
	}
	if len(xs) < 8 {
		return append(xs, s)
	}
	return xs
}
func (a *accessRecorder) record(host, ip string, now time.Time) { a.recordName(host, ip, now, false) }
func (a *accessRecorder) recordName(host, ip string, now time.Time, outer bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	group := host
	if group == "" {
		group = ip
	}
	idx := -1
	for i := range a.rows {
		if a.rows[i].Group == group && a.rows[i].Outer == outer && now.Sub(a.rows[i].seen) < 5*time.Minute {
			idx = i
			break
		}
	}
	if idx < 0 {
		if len(a.rows) >= 100 {
			a.rows = a.rows[1:]
		}
		a.rows = append(a.rows, accessRow{Group: group, Outer: outer, First: a.stamp(now), seen: now})
		idx = len(a.rows) - 1
	}
	r := &a.rows[idx]
	r.Last = a.stamp(now)
	r.Connections++
	r.IPs = boundedAppend(r.IPs, ip)
	if host != "" {
		r.Domains = boundedAppend(r.Domains, host)
	}
}
func (a *accessRecorder) serve() (func(), error) {
	os.Remove(accessSocket)
	l, e := net.Listen("unix", accessSocket)
	if e != nil {
		return func() {}, e
	}
	if e = os.Chmod(accessSocket, 0600); e != nil {
		l.Close()
		return func() {}, e
	}
	go func() {
		for {
			c, e := l.Accept()
			if e != nil {
				return
			}
			c.SetDeadline(time.Now().Add(time.Second))
			a.mu.Lock()
			data, _ := json.Marshal(a.rows)
			a.mu.Unlock()
			c.Write(data)
			c.Close()
		}
	}()
	return func() { l.Close(); os.Remove(accessSocket) }, nil
}
func readAccess() ([]accessRow, bool) {
	c, e := net.DialTimeout("unix", accessSocket, time.Second)
	if e != nil {
		return nil, false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(time.Second))
	var rows []accessRow
	e = json.NewDecoder(io.LimitReader(c, 256<<10)).Decode(&rows)
	return rows, e == nil
}
func (a *accessRecorder) observe(p []byte, now time.Time) {
	if len(p) < 20 || p[0]>>4 != 4 {
		return
	}
	h := int(p[0]&15) * 4
	if h < 20 || len(p) < h+8 || binary.BigEndian.Uint16(p[6:8])&0x3fff != 0 {
		return
	}
	proto := p[9]
	if proto != 6 && proto != 17 {
		return
	}
	port := binary.BigEndian.Uint16(p[h+2 : h+4])
	if port == 53 {
		return
	} // exclude tunnel health/DNS traffic
	var key [13]byte
	copy(key[:8], p[12:20])
	copy(key[8:12], p[h:h+4])
	key[12] = proto
	f := a.flows[key]
	if proto == 6 && len(p) >= h+20 && p[h+13]&2 != 0 {
		if f != nil {
			a.buffered -= len(f.data)
		}
		delete(a.flows, key)
		return
	}
	if f != nil && !f.done && now.Sub(f.seen) > 2*time.Minute {
		if f != nil {
			a.buffered -= len(f.data)
		}
		delete(a.flows, key)
		f = nil
	}
	if f == nil {
		if len(a.flows) >= 1024 {
			var oldest [13]byte
			var stamp time.Time
			count := 0
			for k, v := range a.flows {
				if stamp.IsZero() || v.seen.Before(stamp) {
					oldest = k
					stamp = v.seen
				}
				count++
				if count >= 32 {
					break
				}
			}
			a.buffered -= len(a.flows[oldest].data)
			delete(a.flows, oldest)
		}
		f = &accessFlow{seen: now}
		a.flows[key] = f
	}
	if f.done {
		if now.Sub(f.seen) >= 10*time.Second {
			a.recordName(f.host, net.IP(p[16:20]).String(), now, f.outer)
			f.seen = now
		}
		return
	}
	ip := net.IP(p[16:20]).String()
	if proto == 17 {
		f.done = true
		a.record("", ip, now)
		return
	}
	if len(p) < h+20 {
		return
	}
	th := int(p[h+12]>>4) * 4
	if th < 20 || len(p) < h+th {
		return
	}
	body := p[h+th:]
	if len(body) == 0 {
		return
	}
	seq := binary.BigEndian.Uint32(p[h+4 : h+8])
	if len(f.data) == 0 {
		f.seq = seq
	}
	// Never guess across gaps or out-of-order payloads; retransmissions are ignored.
	if seq < f.seq {
		return
	}
	if seq != f.seq {
		f.done = true
		a.buffered -= len(f.data)
		f.data = nil
		a.record("", ip, now)
		return
	}
	if len(f.data)+len(body) > 16384 || a.buffered+len(body) > 1<<20 {
		f.done = true
		a.buffered -= len(f.data)
		f.data = nil
		a.record("", ip, now)
		return
	}
	f.data = append(f.data, body...)
	a.buffered += len(body)
	f.seq += uint32(len(body))
	host, complete, outer := accessHostInfo(f.data)
	if complete {
		f.host = host
		f.outer = outer
		f.done = true
		a.buffered -= len(f.data)
		f.data = nil
		a.recordName(host, ip, now, outer)
	}
}

// Parse only cleartext HTTP Host or a standard TLS ClientHello SNI. ECH outer
// names are explicitly labeled as outer names, never asserted as visited sites.
func accessHost(b []byte) (string, bool) { h, c, _ := accessHostInfo(b); return h, c }
func accessHostInfo(b []byte) (string, bool, bool) {
	if len(b) < 5 {
		return "", false, false
	}
	if b[0] != 22 {
		s := string(b)
		if !(strings.HasPrefix(s, "GET ") || strings.HasPrefix(s, "POST ") || strings.HasPrefix(s, "HEAD ") || strings.HasPrefix(s, "PUT ") || strings.HasPrefix(s, "DELETE ") || strings.HasPrefix(s, "OPTIONS ") || strings.HasPrefix(s, "PATCH ") || strings.HasPrefix(s, "CONNECT ")) {
			return "", true, false
		}
		end := strings.Index(s, "\r\n\r\n")
		if end < 0 && len(b) < 8192 {
			return "", false, false
		}
		for _, line := range strings.Split(s, "\r\n") {
			if strings.HasPrefix(strings.ToLower(line), "host:") {
				v := strings.TrimSpace(line[5:])
				if h, _, e := net.SplitHostPort(v); e == nil {
					v = h
				}
				v = strings.ToLower(v)
				if validDomain(v) {
					return v, true, false
				}
			}
		}
		return "", true, false
	}
	n := int(binary.BigEndian.Uint16(b[3:5]))
	if n > 16379 {
		return "", true, false
	}
	if len(b) < 5+n {
		return "", false, false
	}
	b = b[5 : 5+n]
	if len(b) < 39 || b[0] != 1 {
		return "", true, false
	}
	i := 38
	i += 1 + int(b[i])
	if i+2 > len(b) {
		return "", true, false
	}
	i += 2 + int(binary.BigEndian.Uint16(b[i:i+2]))
	if i >= len(b) {
		return "", true, false
	}
	i += 1 + int(b[i])
	if i+2 > len(b) {
		return "", true, false
	}
	end := i + 2 + int(binary.BigEndian.Uint16(b[i:i+2]))
	i += 2
	if end > len(b) {
		return "", true, false
	}
	host := ""
	outer := false
	for i+4 <= end {
		typ := binary.BigEndian.Uint16(b[i : i+2])
		size := int(binary.BigEndian.Uint16(b[i+2 : i+4]))
		i += 4
		if i+size > end {
			return "", true, false
		}
		v := b[i : i+size]
		i += size
		if typ == 0xfe0d {
			outer = true
		}
		if typ == 0 && len(v) >= 5 && v[2] == 0 {
			l := int(binary.BigEndian.Uint16(v[3:5]))
			if l <= len(v)-5 {
				s := strings.ToLower(string(v[5 : 5+l]))
				if validDomain(s) {
					host = s
				}
			}
		}
	}
	return host, true, outer
}

// OpenWrt usually has POSIX /etc/TZ but no Go zoneinfo database. Read the OS
// offset once when the tunnel starts, never invoke a process per packet.
func routerLocation() *time.Location {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	b, e := exec.CommandContext(ctx, "date", "+%z").Output()
	if e != nil {
		return time.Local
	}
	return offsetLocation(strings.TrimSpace(string(b)))
}
func offsetLocation(s string) *time.Location {
	if len(s) != 5 || (s[0] != '+' && s[0] != '-') {
		return time.Local
	}
	h, e := strconv.Atoi(s[1:3])
	m, e2 := strconv.Atoi(s[3:5])
	if e != nil || e2 != nil || h > 23 || m > 59 {
		return time.Local
	}
	offset := (h*60 + m) * 60
	if s[0] == '-' {
		offset = -offset
	}
	return time.FixedZone(s, offset)
}
func (a *accessRecorder) stamp(t time.Time) string {
	if a.zone != nil {
		t = t.In(a.zone)
	}
	return t.Format("2006-01-02 15:04:05")
}
