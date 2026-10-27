package main

import "testing"

func TestBlacklistValidation(t *testing.T) {
	d, n, e := parseBlacklist("Example.COM\n192.0.2.3\n192.0.2.18/24\nexample.com")
	if e != nil || len(d) != 1 || d[0] != "example.com" || len(n) != 2 {
		t.Fatal(d, n, e)
	}
	for _, s := range []string{"https://example.com", "example.com;flush", "999.2.3.4", "0.0.0.0/0", "::1"} {
		if _, _, e := parseBlacklist(s); e == nil {
			t.Fatal(s)
		}
	}
}
