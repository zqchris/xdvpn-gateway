package main

import (
	"os"
	"runtime/debug"
	"testing"
	"time"
)

func TestTUNPollingAcrossGC(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("requires root and /dev/net/tun")
	}
	if _, e := os.Stat("/dev/net/tun"); e != nil {
		t.Skip("requires TUN")
	}
	f, e := openTUN("xdvpn-tuntest")
	if e != nil {
		t.Fatal(e)
	}
	defer f.Close()
	debug.FreeOSMemory()
	if e = f.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); e != nil {
		t.Fatal(e)
	}
	_, e = f.Read(make([]byte, 2048))
	if e != nil && !os.IsTimeout(e) {
		t.Fatal(e)
	}
}
