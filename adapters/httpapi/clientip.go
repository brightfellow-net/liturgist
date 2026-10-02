// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// ClientIP implements the client-IP algorithm of 03 §5. warn is called when
// proxy headers arrive from an address that isn't a trusted proxy.
func ClientIP(r *http.Request, trusted []netip.Prefix, header string, warn func()) netip.Addr {
	remote := remoteAddr(r)
	if !isTrusted(remote, trusted) {
		if warn != nil && (r.Header.Get("X-Forwarded-For") != "" || (header != "" && r.Header.Get(header) != "")) {
			warn()
		}
		return remote
	}
	if header != "" {
		vals := r.Header.Values(header)
		if len(vals) == 1 {
			if a, err := netip.ParseAddr(strings.TrimSpace(vals[0])); err == nil {
				return a.Unmap()
			}
		}
		return remote
	}
	parts := strings.Split(strings.Join(r.Header.Values("X-Forwarded-For"), ","), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		p := strings.TrimSpace(parts[i])
		if p == "" && len(parts) == 1 {
			return remote // no header at all
		}
		a, err := netip.ParseAddr(p)
		if err != nil {
			return remote // malformed element on the walk: fall back
		}
		if a = a.Unmap(); !isTrusted(a, trusted) {
			return a
		}
	}
	return remote
}

func remoteAddr(r *http.Request) netip.Addr {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	a, err := netip.ParseAddr(host)
	if err != nil {
		return netip.IPv4Unspecified()
	}
	return a.Unmap()
}

func isTrusted(a netip.Addr, trusted []netip.Prefix) bool {
	for _, p := range trusted {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
