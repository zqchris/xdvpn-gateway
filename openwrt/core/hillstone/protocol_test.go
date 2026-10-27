// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
)

type fragmented struct{ r io.Reader }

func (f fragmented) Read(b []byte) (int, error) {
	if len(b) > 1 {
		b = b[:1]
	}
	return f.r.Read(b)
}
func TestFragmentedMessages(t *testing.T) {
	b := packet(3, field{7, []byte{10, 1, 2, 3}}, field{17, []byte{10, 1, 2, 53}}, field{84, []byte("odd")})
	m, e := readMessage(fragmented{bytes.NewReader(b)})
	if e != nil || m.kind != 3 || len(m.fields[84]) != 3 {
		t.Fatal(e)
	}
}
func TestMalformedFrames(t *testing.T) {
	good := packet(1, num(4, 0, 4))
	for _, which := range []string{"magic", "length", "short", "duplicate", "padding"} {
		b := append([]byte(nil), good...)
		switch which {
		case "magic":
			b[0] = 1
		case "length":
			binary.BigEndian.PutUint32(b[4:], 65537)
		case "short":
			b = b[:7]
		case "duplicate":
			b = packet(1, num(4, 0, 4), num(4, 0, 4))
		case "padding":
			b[11] = 25
		}
		if _, e := readMessage(bytes.NewReader(b)); e == nil {
			t.Fatal(which)
		}
	}
}
func TestAuthRequiresExplicitSuccess(t *testing.T) {
	for _, b := range [][]byte{packet(1), packet(1, num(4, 3, 4)), packet(2, num(4, 0, 4))} {
		b[1] = 2
		if _, e := expect(bytes.NewReader(b), 1); e == nil {
			t.Fatal("accepted invalid auth response")
		}
	}
}
