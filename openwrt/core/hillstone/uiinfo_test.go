package main

import (
	"encoding/json"
	"net"
	"strings"
	"testing"
)

func TestActivitySanitization(t *testing.T) {
	for _, line := range []string{"Mon Oct 26 07:00:00 2026 netifd: xdvpn: password=SECRET", "Mon Oct 26 07:00:00 2026 netifd: xdvpn: SCVPN stats tx=1"} {
		if _, ok := activityLine(line); ok {
			t.Fatal("raw unrelated event exposed")
		}
	}
	e, ok := activityLine("Mon Oct 26 07:00:00 2026 netifd: xdvpn: hillstone: SECRET password")
	if !ok || strings.Contains(e.Text, "SECRET") {
		t.Fatal(e)
	}
	e, ok = activityLine("Mon Oct 26 07:00:00 2026 netifd: Interface 'xdvpn' is now down")
	if !ok || e.Kind != "disconnected" {
		t.Fatal(e)
	}
}
func TestNftAddressMembership(t *testing.T) {
	for _, s := range []string{`"10.1.2.3"`, `{"prefix":{"addr":"10.1.0.0","len":16}}`, `{"range":["10.1.2.1","10.1.2.9"]}`, `{"elem":{"val":"10.1.2.3","expires":1000}}`} {
		var v any
		if e := json.Unmarshal([]byte(s), &v); e != nil {
			t.Fatal(e)
		}
		if !setHasIP(v, net.ParseIP("10.1.2.3")) || setHasIP(v, net.ParseIP("10.2.2.3")) {
			t.Fatal(s)
		}
	}
}
func TestBoundedOutput(t *testing.T) {
	b := &cappedOutput{max: 5}
	b.Write([]byte("1234"))
	b.Write([]byte("5678"))
	if string(b.data) != "45678" {
		t.Fatal(string(b.data))
	}
}
