package main

import (
	"context"
	"errors"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestDomainRoutingRefresh(t *testing.T) {
	d, e := newDomainRoutes("service.example.com service.example.com", net.ParseIP("203.0.113.8"))
	if e != nil {
		t.Fatal(e)
	}
	now := time.Now()
	lookup := func(context.Context, string, string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("10.2.3.4"), net.ParseIP("127.0.0.1"), net.ParseIP("203.0.113.8"), net.ParseIP("::1")}, nil
	}
	got, _ := d.refresh(context.Background(), lookup, now)
	if !reflect.DeepEqual(got, []string{"10.2.3.4"}) {
		t.Fatal(got)
	}
	fail := func(context.Context, string, string) ([]net.IP, error) { return nil, errors.New("timeout") }
	got, n := d.refresh(context.Background(), fail, now.Add(time.Minute))
	if len(got) != 1 || n != 1 {
		t.Fatal(got, n)
	}
	got, _ = d.refresh(context.Background(), fail, now.Add(11*time.Minute))
	if len(got) != 0 {
		t.Fatal("stale entries retained", got)
	}
	changed := func(context.Context, string, string) ([]net.IP, error) { return []net.IP{net.ParseIP("10.2.3.5")}, nil }
	got, _ = d.refresh(context.Background(), changed, now.Add(12*time.Minute))
	if !reflect.DeepEqual(got, []string{"10.2.3.5"}) {
		t.Fatal(got)
	}
}
func TestInvalidDomain(t *testing.T) {
	for _, s := range []string{"https://example.com", "*.example.com", "127.0.0.1", "a..com", "-a.com", "a.com/path", "a.com;id"} {
		if _, e := newDomainRoutes(s, nil); e == nil {
			t.Fatal(s)
		}
	}
}
