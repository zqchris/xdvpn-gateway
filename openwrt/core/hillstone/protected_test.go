package main

import "testing"

func TestProtectedSuffixBoundary(t *testing.T) {
	for _, s := range []string{"api.anthropic.com", "CLAUDE.AI.", "x.frame.claudeusercontent.com", "claude.com"} {
		if !protectedDomain(s, protectedSuffixes) {
			t.Fatal(s)
		}
	}
	for _, s := range []string{"anthropic.com.attacker.test", "notanthropic.com", "company.example.com"} {
		if protectedDomain(s, protectedSuffixes) {
			t.Fatal(s)
		}
	}
}
