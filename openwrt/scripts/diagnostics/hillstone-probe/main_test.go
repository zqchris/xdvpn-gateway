package main

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func TestReply(t *testing.T) {
	b := packet(2, field{4, []byte{255, 255, 255, 255}})
	b[1] = 2
	s, e := reply(bytes.NewReader(b), 2)
	if e != nil || s != 0xffffffff {
		t.Fatalf("%d %v", s, e)
	}
}
func TestRejectMalformed(t *testing.T) {
	good := packet(2, field{4, []byte{0, 0, 0, 0}})
	good[1] = 2
	for _, kind := range []string{"short", "oversize", "wrong-type", "duplicate", "truncated-field"} {
		b := append([]byte(nil), good...)
		switch kind {
		case "short":
			b = b[:6]
		case "oversize":
			binary.BigEndian.PutUint32(b[4:8], 65537)
		case "wrong-type":
			b[3] = 1
		case "duplicate":
			b = packet(2, field{4, []byte{0, 0, 0, 0}}, field{4, []byte{0, 0, 0, 0}})
			b[1] = 2
		case "truncated-field":
			b[11] = 20
		}
		if _, e := reply(bytes.NewReader(b), 2); e == nil {
			t.Error(kind)
		}
	}
}
func TestPadding(t *testing.T) {
	b := packet(1, field{84, []byte("abc")}, field{81, []byte{0, 1}})
	if len(b) != 24 || b[15] != 0 || binary.BigEndian.Uint16(b[16:18]) != 81 {
		t.Fatal("invalid alignment")
	}
}
