//go:build !linux

// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"crypto/tls"
	"errors"
	"net"
	"time"
)

func dataTunnel(c *tls.Conn, config, k message, key, spi []byte, remote net.IP, port, enc, auth uint32, duration time.Duration) error {
	return errors.New("tunnel requires Linux")
}

func runOpenConnectPolicy(watch bool) error { return errors.New("policy requires Linux") }
