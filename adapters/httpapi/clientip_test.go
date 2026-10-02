// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

// TC-A-011
func TestClientIP(t *testing.T) {
	trusted := []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32"), netip.MustParsePrefix("10.0.0.0/8")}
	cases := []struct {
		name, remote, header string
		xff                  []string
		hdr                  map[string][]string
		want                 string
		warns                bool
	}{
		{name: "trusted proxy, single client", remote: "127.0.0.1:5000", xff: []string{"203.0.113.5"}, want: "203.0.113.5"},
		{name: "chain through trusted proxies", remote: "127.0.0.1:5000", xff: []string{"203.0.113.5, 10.1.2.3"}, want: "203.0.113.5"},
		{name: "multiple header lines", remote: "127.0.0.1:5000", xff: []string{"198.51.100.1", "203.0.113.5"}, want: "203.0.113.5"},
		{name: "spoofed left part ignored", remote: "127.0.0.1:5000", xff: []string{"1.2.3.4, 203.0.113.5"}, want: "203.0.113.5"},
		{name: "malformed element falls back", remote: "127.0.0.1:5000", xff: []string{"unknown, 10.1.2.3"}, want: "127.0.0.1"},
		{name: "port in element falls back", remote: "127.0.0.1:5000", xff: []string{"203.0.113.5:443"}, want: "127.0.0.1"},
		{name: "all trusted falls back", remote: "127.0.0.1:5000", xff: []string{"10.1.2.3"}, want: "127.0.0.1"},
		{name: "untrusted remote ignores header and warns", remote: "198.51.100.9:5000", xff: []string{"203.0.113.5"}, want: "198.51.100.9", warns: true},
		{name: "no headers", remote: "198.51.100.9:5000", want: "198.51.100.9"},
		{name: "custom header", remote: "127.0.0.1:5000", header: "CF-Connecting-IP",
			hdr: map[string][]string{"Cf-Connecting-Ip": {"203.0.113.7"}}, xff: []string{"9.9.9.9"}, want: "203.0.113.7"},
		{name: "custom header twice falls back", remote: "127.0.0.1:5000", header: "CF-Connecting-IP",
			hdr: map[string][]string{"Cf-Connecting-Ip": {"203.0.113.7", "203.0.113.8"}}, want: "127.0.0.1"},
		{name: "IPv4-mapped IPv6", remote: "[::ffff:198.51.100.9]:5000", want: "198.51.100.9"},
	}
	for _, c := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = c.remote
		for _, v := range c.xff {
			r.Header.Add("X-Forwarded-For", v)
		}
		for k, vs := range c.hdr {
			for _, v := range vs {
				r.Header.Add(k, v)
			}
		}
		warned := false
		got := ClientIP(r, trusted, c.header, func() { warned = true })
		if got.String() != c.want || warned != c.warns {
			t.Errorf("%s: got %s warned=%v, want %s warned=%v", c.name, got, warned, c.want, c.warns)
		}
	}
}
