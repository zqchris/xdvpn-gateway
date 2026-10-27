package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreCompanyRulesAtomicValidation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "company-rules.txt")
	original := "192.0.2.1\nexample.com\n"
	r, e := storeCompanyRules(path, original)
	if e != nil || len(r.Networks) != 1 || len(r.Domains) != 1 {
		t.Fatal(r, e)
	}
	st, _ := os.Stat(path)
	if st.Mode().Perm() != 0600 {
		t.Fatal("private permissions", st.Mode())
	}
	for _, invalid := range []string{"", "https://example.com", strings.Repeat("a", 262145)} {
		if _, e = storeCompanyRules(path, invalid); e == nil {
			t.Fatal("accepted invalid input")
		}
		b, _ := os.ReadFile(path)
		if string(b) != original {
			t.Fatal("invalid import changed original")
		}
	}
	if _, e = storeCompanyRules(path, "198.51.100.2\nchild.example.org\n"); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "example.com") {
		t.Fatal("did not replace")
	}
	files, _ := filepath.Glob(filepath.Join(filepath.Dir(path), ".company-rules-*"))
	if len(files) != 0 {
		t.Fatal("temporary files leaked")
	}
}
