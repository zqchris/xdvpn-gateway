package main

import (
	"strings"
	"testing"
)

func TestCompanyRules(t *testing.T) {
	r, e := parseCompanyRules(strings.NewReader("\ufeff# IT list\n192.0.2.3，\n198.51.100.18/24, example.com # note\n192.0.2.3\nChild.Example.com\r\n"))
	if e != nil || len(r.Networks) != 2 || len(r.Domains) != 2 || r.Networks[1] != "198.51.100.0/24" {
		t.Fatal("normalization failed", e)
	}
	for _, bad := range []string{"", "# empty", "192.0.2.3\ninvalid!", "999.0.0.1", "0.0.0.0/0", "::1", "https://example.com"} {
		if _, e := parseCompanyRules(strings.NewReader(bad)); e == nil {
			t.Fatal("invalid rule accepted")
		}
	}
	if _, e := parseCompanyRules(strings.NewReader("invalid-sensitive-value")); e == nil || strings.Contains(e.Error(), "sensitive") {
		t.Fatal("error must not disclose values")
	}
}
func TestPolicyModes(t *testing.T) {
	if policyMode("server") != "company" {
		t.Fatal("legacy server must never import push")
	}
	d, n, e := policyTargets("networks", "example.com")
	if e != nil || d != "example.com" || len(n) != 0 {
		t.Fatal("manual must not require preset")
	}
	d, n, e = policyTargets("global4", "example.com")
	if e != nil || d != "" || len(n) != 0 {
		t.Fatal("global must not require preset")
	}
}
