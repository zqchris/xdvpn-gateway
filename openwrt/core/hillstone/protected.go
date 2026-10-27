// SPDX-License-Identifier: GPL-3.0-only
package main

import "strings"

var protectedSuffixes = []string{"anthropic.com", "claude.ai", "claude.com", "claude.app", "claudeusercontent.com", "claudemcpcontent.com"}

func protectedDomain(name string, suffixes []string) bool {
	name = strings.ToLower(strings.TrimSuffix(name, "."))
	for _, suffix := range suffixes {
		if name == suffix || strings.HasSuffix(name, "."+suffix) {
			return true
		}
	}
	return false
}
