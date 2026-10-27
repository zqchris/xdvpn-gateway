package main

import (
	"encoding/binary"
	"reflect"
	"testing"
)

func TestServerRouteModes(t *testing.T) {
	b := make([]byte, 24)
	binary.BigEndian.PutUint32(b[4:8], 0x80000000)
	binary.BigEndian.PutUint32(b[8:12], 35)
	binary.BigEndian.PutUint32(b[12:16], 0x80000000)
	binary.BigEndian.PutUint32(b[16:20], 0x80000000)
	binary.BigEndian.PutUint32(b[20:24], 35)
	got, e := serverRoutes(b)
	if e != nil || !reflect.DeepEqual(got, []string{"0.0.0.0/1", "128.0.0.0/1"}) {
		t.Fatal(got, e)
	}
	if _, e = serverRoutes(b[:23]); e == nil {
		t.Fatal("truncated route accepted")
	}
	b[5] = 1
	if _, e = serverRoutes(b); e == nil {
		t.Fatal("invalid mask accepted")
	}
	for _, mode := range []string{"networks", "server"} {
		if !protectEnabled(mode, "") || !protectEnabled(mode, "1") || protectEnabled(mode, "0") {
			t.Fatal(mode)
		}
	}
	if protectEnabled("global4", "1") {
		t.Fatal("global mode retained exclusions")
	}
}
func TestOpenConnectRoutes(t *testing.T) {
	d := map[string]string{"CISCO_SPLIT_INC": "1", "CISCO_SPLIT_INC_0_ADDR": "10.20.30.0", "CISCO_SPLIT_INC_0_MASKLEN": "24", "CISCO_SPLIT_EXC": "1", "CISCO_SPLIT_EXC_0_ADDR": "10.20.30.9", "CISCO_SPLIT_EXC_0_MASKLEN": "32"}
	get := func(k string) string { return d[k] }
	inc, e := openConnectRoutes(get, "CISCO_SPLIT_INC", true)
	if e != nil || inc[0] != "10.20.30.0/24" {
		t.Fatal(inc, e)
	}
	exc, e := openConnectRoutes(get, "CISCO_SPLIT_EXC", false)
	if e != nil || exc[0] != "10.20.30.9/32" {
		t.Fatal(exc, e)
	}
	delete(d, "CISCO_SPLIT_INC")
	inc, e = openConnectRoutes(get, "CISCO_SPLIT_INC", true)
	if e != nil || inc[0] != "0.0.0.0/0" {
		t.Fatal(inc, e)
	}
	d["CISCO_SPLIT_INC"] = "1"
	d["CISCO_SPLIT_INC_0_MASKLEN"] = "999"
	if _, e = openConnectRoutes(get, "CISCO_SPLIT_INC", true); e == nil {
		t.Fatal("invalid route accepted")
	}
}
