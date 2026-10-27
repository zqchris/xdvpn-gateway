package main

import (
	"bytes"
	"crypto/aes"
	"crypto/hmac"
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"testing"
)

func testSA() *securityAssociation {
	b, _ := aes.NewCipher(make([]byte, 16))
	return &securityAssociation{block: b, mac: hmac.New(md5.New, make([]byte, 16)), spi: 123, tag: 12}
}
func TestESPIntegrityReplayAndRoundtrip(t *testing.T) {
	out, in := testSA(), testSA()
	ip := make([]byte, 43)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], uint16(len(ip)))
	p, e := out.encrypt(make([]byte, 2048), ip)
	if e != nil {
		t.Fatal(e)
	}
	saved := append([]byte(nil), p...)
	bad := append([]byte(nil), p...)
	bad[30] ^= 1
	if _, e = in.decrypt(bad); e == nil {
		t.Fatal("accepted tampered packet")
	}
	r, e := in.decrypt(p)
	if e != nil || !bytes.Equal(ip, r) {
		t.Fatalf("roundtrip: %v", e)
	}
	if _, e = in.decrypt(saved); e == nil {
		t.Fatal("accepted replay")
	}
}
func TestESPRejectMalformed(t *testing.T) {
	for n := 0; n < 60; n++ {
		if _, e := testSA().decrypt(make([]byte, n)); e == nil {
			t.Fatalf("accepted length %d", n)
		}
	}
}
func TestKDFDirection(t *testing.T) {
	in, out, e := associations(make([]byte, 48), 1, 2, 12, 1)
	if e != nil {
		t.Fatal(e)
	}
	if in.spi != 1 || out.spi != 2 {
		t.Fatal("wrong SPI mapping")
	}
	if _, _, e = associations(nil, 1, 2, 3, 2); e == nil {
		t.Fatal("unexpected legacy cipher support")
	}
}
func BenchmarkEncrypt(b *testing.B) {
	s := testSA()
	ip := make([]byte, 1380)
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], 1380)
	dst := make([]byte, 2048)
	b.SetBytes(1380)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, e := s.encrypt(dst, ip); e != nil {
			b.Fatal(e)
		}
	}
}

// Public interoperability vector from NolanHo/hilldust selftest.py (GPL-3.0).
func TestESPIndependentKnownAnswer(t *testing.T) {
	ck, ak := make([]byte, 16), make([]byte, 16)
	for i := range ck {
		ck[i] = byte(i)
		ak[i] = byte(i + 16)
	}
	block, _ := aes.NewCipher(ck)
	sa := &securityAssociation{block: block, mac: hmac.New(md5.New, ak), spi: 0x11223344, tag: 12}
	wire, _ := hex.DecodeString("1122334400000001202122232425262728292a2b2c2d2e2fd2633873268769d3eb79f23cd917d95316dad0fef65be9f637964ae009d61bb8f057207e42e4a4bf6769165f904df4ee99466721fcd8b88ec56e276691b486e1543e22fb2f9adc5517e9b466")
	want, _ := hex.DecodeString("4500003c0001000040115c9d0a0101010a09090904d200350028cc730101010101010101010101010101010101010101010101010101010101010101")
	got, e := sa.decrypt(wire)
	if e != nil || !bytes.Equal(got, want) {
		t.Fatalf("independent vector: %v, bytes=%d", e, len(got))
	}
}
func TestEveryPaddingResidue(t *testing.T) {
	out, in := testSA(), testSA()
	for n := 20; n < 1500; n++ {
		ip := make([]byte, n)
		ip[0] = 0x45
		binary.BigEndian.PutUint16(ip[2:4], uint16(n))
		p, e := out.encrypt(make([]byte, 2048), ip)
		if e != nil {
			t.Fatal(e)
		}
		r, e := in.decrypt(p)
		if e != nil || !bytes.Equal(r, ip) {
			t.Fatalf("length=%d error=%v", n, e)
		}
	}
}
