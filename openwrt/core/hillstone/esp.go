// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/md5"
	"crypto/rand"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/binary"
	"errors"
	"hash"
	"math"
)

type resetCBC interface {
	cipher.BlockMode
	SetIV([]byte)
}
type securityAssociation struct {
	encryptor         resetCBC
	decryptor         resetCBC
	digest            [64]byte
	block             cipher.Block
	mac               hash.Hash
	spi, seq, highest uint32
	window            uint64
	tag               int
}

func keySizes(enc, auth uint32) (int, int, int, func() hash.Hash, error) {
	var ck, ak, tag int
	var hf func() hash.Hash
	switch enc {
	case 12:
		ck = 16
	case 14:
		ck = 24
	case 15:
		ck = 32
	default:
		return 0, 0, 0, nil, errors.New("unsupported encryption algorithm")
	}
	switch auth {
	case 1:
		ak = 16
		tag = 12
		hf = md5.New
	case 2:
		ak = 20
		tag = 12
		hf = sha1.New
	case 5:
		ak = 32
		tag = 16
		hf = sha256.New
	case 6:
		ak = 48
		tag = 24
		hf = sha512.New384
	case 7:
		ak = 64
		tag = 32
		hf = sha512.New
	default:
		return 0, 0, 0, nil, errors.New("unsupported integrity algorithm")
	}
	return ck, ak, tag, hf, nil
}
func associations(key []byte, inSPI, outSPI, enc, auth uint32) (*securityAssociation, *securityAssociation, error) {
	ck, ak, tag, hf, e := keySizes(enc, auth)
	if e != nil {
		return nil, nil, e
	}
	first := sha1.Sum(key)
	derived := append([]byte(nil), first[:]...)
	for len(derived) < 2*(ck+ak) {
		h := sha1.Sum(derived)
		derived = append(derived, h[:]...)
	}
	defer clear(derived)
	makeSA := func(spi uint32, offset int) (*securityAssociation, error) {
		b, e := aes.NewCipher(derived[offset+ak : offset+ak+ck])
		if e != nil {
			return nil, e
		}
		return &securityAssociation{block: b, mac: hmac.New(hf, derived[offset:offset+ak]), spi: spi, tag: tag}, nil
	}
	out, e := makeSA(outSPI, 0)
	if e != nil {
		return nil, nil, e
	}
	in, e := makeSA(inSPI, ak+ck)
	return in, out, e
}
func (s *securityAssociation) encrypt(dst, payload []byte) ([]byte, error) {
	if s.seq == math.MaxUint32 {
		return nil, errors.New("ESP sequence exhausted; reconnect required")
	}
	if len(payload) < 20 || payload[0]>>4 != 4 {
		return nil, errors.New("IPv4 required")
	}
	bs := s.block.BlockSize()
	pad := (bs - (len(payload)+2)%bs) % bs
	n := 8 + bs + len(payload) + pad + 2 + s.tag
	if cap(dst) < n {
		return nil, errors.New("ESP buffer too small")
	}
	dst = dst[:n]
	s.seq++
	binary.BigEndian.PutUint32(dst, s.spi)
	binary.BigEndian.PutUint32(dst[4:], s.seq)
	iv := dst[8 : 8+bs]
	if _, e := rand.Read(iv); e != nil {
		return nil, e
	}
	data := dst[8+bs : n-s.tag]
	copy(data, payload)
	for i := 0; i < pad; i++ {
		data[len(payload)+i] = byte(i + 1)
	}
	data[len(data)-2] = byte(pad)
	data[len(data)-1] = 4
	if s.encryptor == nil {
		s.encryptor = cipher.NewCBCEncrypter(s.block, iv).(resetCBC)
	} else {
		s.encryptor.SetIV(iv)
	}
	s.encryptor.CryptBlocks(data, data)
	s.mac.Reset()
	s.mac.Write(dst[:n-s.tag])
	sum := s.mac.Sum(s.digest[:0])
	copy(dst[n-s.tag:], sum[:s.tag])
	return dst, nil
}
func (s *securityAssociation) decrypt(b []byte) ([]byte, error) {
	bs := s.block.BlockSize()
	if len(b) < 8+bs+bs+s.tag || (len(b)-8-bs-s.tag)%bs != 0 {
		return nil, errors.New("invalid ESP size")
	}
	if binary.BigEndian.Uint32(b) != s.spi {
		return nil, errors.New("ESP SPI mismatch")
	}
	seq := binary.BigEndian.Uint32(b[4:])
	if seq == 0 || seq <= s.highest && (s.highest-seq >= 64 || s.window&(uint64(1)<<(s.highest-seq)) != 0) {
		return nil, errors.New("ESP replay")
	}
	end := len(b) - s.tag
	s.mac.Reset()
	s.mac.Write(b[:end])
	sum := s.mac.Sum(s.digest[:0])
	if !hmac.Equal(sum[:s.tag], b[end:]) {
		return nil, errors.New("ESP integrity failure")
	}
	data := b[8+bs : end]
	if s.decryptor == nil {
		s.decryptor = cipher.NewCBCDecrypter(s.block, b[8:8+bs]).(resetCBC)
	} else {
		s.decryptor.SetIV(b[8 : 8+bs])
	}
	s.decryptor.CryptBlocks(data, data)
	pad := int(data[len(data)-2])
	if pad+2 > len(data) || data[len(data)-1] != 4 {
		return nil, errors.New("ESP padding/protocol invalid")
	}
	for i := 1; i <= pad; i++ {
		if data[len(data)-2-pad+i-1] != byte(i) {
			return nil, errors.New("ESP padding invalid")
		}
	}
	data = data[:len(data)-pad-2]
	if len(data) < 20 || data[0]>>4 != 4 || int(binary.BigEndian.Uint16(data[2:4])) != len(data) {
		return nil, errors.New("invalid inner IPv4 packet")
	}
	if seq > s.highest {
		d := seq - s.highest
		if d >= 64 {
			s.window = 1
		} else {
			s.window = s.window<<d | 1
		}
		s.highest = seq
	} else {
		s.window |= uint64(1) << (s.highest - seq)
	}
	return data, nil
}
