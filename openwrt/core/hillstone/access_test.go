package main

import (
	"encoding/binary"
	"testing"
	"time"
)

func hello(host string) []byte {
	sni := []byte{0, byte(len(host) + 3), 0, 0, byte(len(host))}
	sni = append(sni, host...)
	ext := []byte{0, 0, 0, byte(len(sni))}
	ext = append(ext, sni...)
	b := make([]byte, 38)
	b[0] = 1
	b = append(b, 0, 0, 2, 0x13, 1, 1, 0, 0, byte(len(ext)))
	b = append(b, ext...)
	record := []byte{22, 3, 3, byte(len(b) >> 8), byte(len(b))}
	return append(record, b...)
}
func accessPacket(body []byte, seq uint32) []byte {
	p := make([]byte, 40)
	p[0] = 0x45
	p[9] = 6
	copy(p[12:20], []byte{192, 168, 1, 2, 10, 1, 2, 3})
	p[20] = 0x12
	p[21] = 0x34
	p[22] = 1
	p[23] = 0xbb
	p[32] = 0x50
	binary.BigEndian.PutUint32(p[24:28], seq)
	return append(p, body...)
}
func TestAccessGroupingWindow(t *testing.T) {
	a := newAccessRecorder("example.com")
	now := time.Now()
	a.record("a.example.com", "10.0.0.1", now)
	a.record("b.example.com", "10.0.0.2", now.Add(time.Minute))
	a.record("a.example.com", "10.0.0.3", now.Add(4*time.Minute))
	if len(a.rows) != 2 || a.rows[0].Connections != 2 || a.rows[0].Group != "a.example.com" {
		t.Fatal(a.rows)
	}
	a.record("a.example.com", "10.0.0.1", now.Add(5*time.Minute))
	if len(a.rows) != 3 {
		t.Fatal("fixed window must expire")
	}
	for i := 0; i < 200; i++ {
		a.record("a.example.com", "10.0.0.1", now.Add(time.Duration(i+2)*6*time.Minute))
	}
	if len(a.rows) != 100 {
		t.Fatal(len(a.rows))
	}
}
func TestAccessFragmentedTCP(t *testing.T) {
	a := newAccessRecorder("example.com")
	b := hello("a.example.com")
	now := time.Now()
	a.observe(accessPacket(b[:20], 100), now)
	if len(a.rows) != 0 {
		t.Fatal("premature log")
	}
	a.observe(accessPacket(b[20:], 120), now)
	a.observe(accessPacket(b[20:], 120), now)
	if len(a.rows) != 1 || a.rows[0].Domains[0] != "a.example.com" || a.rows[0].Connections != 1 {
		t.Fatal(a.rows)
	}
}
func TestAccessHost(t *testing.T) {
	b := hello("a.example.com")
	if h, ok := accessHost(b); !ok || h != "a.example.com" {
		t.Fatal(h, ok)
	}
	for i := 0; i < len(b); i++ {
		accessHost(b[:i])
	}
	if h, _ := accessHost([]byte("GET / HTTP/1.1\r\nHost: b.example.com:80\r\n\r\n")); h != "b.example.com" {
		t.Fatal(h)
	}
	// ECH must suppress outer SNI.
	b = append(b, 0xfe, 0x0d, 0, 0)
	binary.BigEndian.PutUint16(b[3:5], uint16(len(b)-5))
	binary.BigEndian.PutUint16(b[50:52], uint16(len(b)-52))
	if h, c, outer := accessHostInfo(b); h != "a.example.com" || !c || !outer {
		t.Fatal("ECH outer name must be labeled", h, c, outer)
	}
}
func FuzzAccessHost(f *testing.F) {
	f.Add(hello("test.example.com"))
	f.Add([]byte("GET / HTTP/1.1\r\nHost: example.com\r\n\r\n"))
	f.Fuzz(func(t *testing.T, b []byte) { accessHost(b); newAccessRecorder("").observe(b, time.Now()) })
}

func TestAccessRouterTime(t *testing.T) {
	a := newAccessRecorder("")
	a.zone = offsetLocation("+0800")
	stamp := a.stamp(time.Date(2026, 10, 25, 23, 36, 0, 0, time.UTC))
	if stamp != "2026-10-26 07:36:00" {
		t.Fatal(stamp)
	}
}

func TestAccessReusedConnectionNewWindow(t *testing.T) {
	a := newAccessRecorder("example.com")
	now := time.Now()
	a.observe(accessPacket(hello("a.example.com"), 100), now)
	a.observe(accessPacket([]byte{23, 3, 3, 0, 0}, 200), now.Add(4*time.Minute))
	if len(a.rows) != 1 {
		t.Fatal(a.rows)
	}
	a.observe(accessPacket([]byte{23, 3, 3, 0, 0}, 205), now.Add(6*time.Minute))
	if len(a.rows) != 2 || a.rows[1].Domains[0] != "a.example.com" {
		t.Fatal(a.rows)
	}
}

func TestAccessFlowBudget(t *testing.T) {
	a := newAccessRecorder("")
	now := time.Now()
	for i := 0; i < 1100; i++ {
		p := accessPacket([]byte{22, 3, 3, 0x30, 0}, 100)
		binary.BigEndian.PutUint16(p[20:22], uint16(i))
		a.observe(p, now)
	}
	if len(a.flows) > 1024 || a.buffered > 1<<20 || a.buffered < 0 {
		t.Fatal(len(a.flows), a.buffered)
	}
}
