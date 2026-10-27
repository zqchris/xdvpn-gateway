package main

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCertificateTrust(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Error("certificate inspection must not send application data")
	}))
	defer server.Close()
	c, err := tls.Dial("tcp", server.Listener.Addr().String(), &tls.Config{InsecureSkipVerify: true})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	state := c.ConnectionState()
	pin := certificateHash(state.PeerCertificates[0])
	if verifyServer(state, "example.com", "") == nil {
		t.Fatal("self signed certificate accepted without confirmation")
	}
	if err := verifyServer(state, "example.com", pin); err != nil {
		t.Fatal(err)
	}
	bad := pin[:63] + "0"
	if bad == pin {
		bad = pin[:63] + "1"
	}
	if verifyServer(state, "example.com", bad) == nil {
		t.Fatal("changed certificate accepted")
	}
	if verifyServer(tls.ConnectionState{}, "example.com", pin) == nil {
		t.Fatal("missing certificate accepted")
	}
}
