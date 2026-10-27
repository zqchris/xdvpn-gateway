// SPDX-License-Identifier: GPL-3.0-only
package main

import (
	"errors"
	"os"
	"syscall"
	"unsafe"
)

func openTUN(name string) (*os.File, error) {
	// Attach before os.NewFile registers the fd with Go's poller. Before
	// TUNSETIFF, /dev/net/tun may report POLLNVAL and poison that registration.
	fd, e := syscall.Open("/dev/net/tun", syscall.O_RDWR|syscall.O_CLOEXEC, 0)
	if e != nil {
		return nil, e
	}
	var ifr [40]byte
	copy(ifr[:16], name)
	*(*uint16)(unsafe.Pointer(&ifr[16])) = 0x1001
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), 0x400454ca, uintptr(unsafe.Pointer(&ifr[0])))
	if errno != 0 {
		syscall.Close(fd)
		return nil, errno
	}
	if e = syscall.SetNonblock(fd, true); e != nil {
		syscall.Close(fd)
		return nil, e
	}
	f := os.NewFile(uintptr(fd), "/dev/net/tun")
	if f == nil {
		syscall.Close(fd)
		return nil, errors.New("cannot wrap TUN fd")
	}
	return f, nil
}
