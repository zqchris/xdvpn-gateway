// SPDX-License-Identifier: GPL-3.0-only
// SCVPN wire-format reference: LionNatsu/hilldust (GPL-3.0), revision 542528d.
package main

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

type field struct {
	kind uint16
	data []byte
}
type message struct {
	kind   uint16
	reply  bool
	fields map[uint16][]byte
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
func readMessage(r io.Reader) (message, error) {
	var m message
	h := make([]byte, 8)
	if _, e := io.ReadFull(r, h); e != nil {
		return m, e
	}
	if string(h) == string(make([]byte, 8)) {
		return m, nil
	}
	n := binary.BigEndian.Uint32(h[4:])
	if h[0] != 0x22 || (h[1] != 0 && h[1] != 2) || n < 8 || n > 65536 {
		return m, errors.New("invalid SCVPN header")
	}
	m.kind = binary.BigEndian.Uint16(h[2:4])
	m.reply = h[1] == 2
	m.fields = make(map[uint16][]byte)
	b := make([]byte, int(n)-8)
	if _, e := io.ReadFull(r, b); e != nil {
		return m, e
	}
	for len(b) > 0 {
		if len(b) < 4 {
			return m, errors.New("truncated field")
		}
		k, l := binary.BigEndian.Uint16(b), int(binary.BigEndian.Uint16(b[2:4]))
		p := (l + 3) &^ 3
		if 4+p > len(b) {
			return m, errors.New("invalid field length")
		}
		if _, ok := m.fields[k]; ok {
			return m, errors.New("duplicate field")
		}
		m.fields[k] = b[4 : 4+l]
		b = b[4+p:]
	}
	return m, nil
}
func (m message) number(k uint16) (uint32, error) {
	v := m.fields[k]
	switch len(v) {
	case 1:
		return uint32(v[0]), nil
	case 2:
		return uint32(binary.BigEndian.Uint16(v)), nil
	case 4:
		return binary.BigEndian.Uint32(v), nil
	}
	return 0, fmt.Errorf("invalid numeric field %d", k)
}

type statusError struct {
	status uint32
	kind   uint16
}

func (e *statusError) Error() string { return fmt.Sprintf("SCVPN status=%d type=%d", e.status, e.kind) }
func (m message) status() error {
	if _, ok := m.fields[4]; !ok {
		return nil
	}
	v, e := m.number(4)
	if e != nil {
		return e
	}
	if v != 0 {
		return &statusError{v, m.kind}
	}
	return nil
}
func num(k uint16, v uint32, n int) field {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, v)
	return field{k, b[4-n:]}
}
func send(w io.Writer, kind uint16, f ...field) error {
	b := packet(kind, f...)
	_, e := w.Write(b)
	clear(b)
	return e
}
func expect(r io.Reader, kind uint16) (message, error) {
	m, e := readMessage(r)
	if e != nil {
		return m, e
	}
	if m.kind != kind || !m.reply {
		return m, fmt.Errorf("unexpected message type=%d", m.kind)
	}
	if _, ok := m.fields[4]; !ok {
		return m, errors.New("missing status")
	}
	return m, m.status()
}
