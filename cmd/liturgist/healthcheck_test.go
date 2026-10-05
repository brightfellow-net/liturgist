// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/brightfellow-net/liturgist/server"
)

func TestHealthURL(t *testing.T) {
	for listen, want := range map[string]string{
		":8080":          "http://127.0.0.1:8080/healthz",
		"0.0.0.0:8080":   "http://127.0.0.1:8080/healthz",
		"[::]:9000":      "http://[::1]:9000/healthz",
		"127.0.0.1:8080": "http://127.0.0.1:8080/healthz",
		"10.0.0.5:80":    "http://10.0.0.5:80/healthz",
	} {
		if got := healthURL(server.Config{Listen: listen}); got != want {
			t.Errorf("healthURL(%q) = %q, want %q", listen, got, want)
		}
	}
}

func envOf(listen string) func(string) string {
	return func(k string) string {
		if k == "LITURGIST_LISTEN" {
			return listen
		}
		return ""
	}
}

func TestHealthcheck(t *testing.T) {
	status := http.StatusOK
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Errorf("path %q", r.URL.Path)
		}
		w.WriteHeader(status)
	}))
	defer ts.Close()
	listen := strings.TrimPrefix(ts.URL, "http://")

	var errb bytes.Buffer
	if code := healthcheck(envOf(listen), &errb); code != exitOK {
		t.Fatalf("healthy server: exit %d, %s", code, errb.String())
	}
	status = http.StatusInternalServerError
	errb.Reset()
	if code := healthcheck(envOf(listen), &errb); code != exitError || !strings.Contains(errb.String(), "500") {
		t.Fatalf("500: exit %d, %q", code, errb.String())
	}
}

func TestHealthcheckNothingListening(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := l.Addr().String()
	_ = l.Close()
	var errb bytes.Buffer
	if code := healthcheck(envOf(addr), &errb); code != exitError || !strings.Contains(errb.String(), "unhealthy") {
		t.Fatalf("exit %d, %q", code, errb.String())
	}
}

func TestHealthcheckBadConfig(t *testing.T) {
	var errb bytes.Buffer
	if code := healthcheck(envOf("nope"), &errb); code != exitError {
		t.Fatalf("exit %d", code)
	}
}

func TestHealthURLWithDomainUsesThePlainPort(t *testing.T) {
	cfg := server.Config{Listen: ":8080", Domain: "liturgi.example.org", HTTPPort: 8081}
	if got, want := healthURL(cfg), "http://127.0.0.1:8081/healthz"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
