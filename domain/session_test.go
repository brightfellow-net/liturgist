// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"net/netip"
	"regexp"
	"testing"
	"time"
)

func TestSessionExpiry(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	ttl, maxAge := 90*24*time.Hour, 365*24*time.Hour
	if got := SessionExpiry(created, created, ttl, maxAge); !got.Equal(created.Add(ttl)) {
		t.Errorf("fresh: %v", got)
	}
	late := created.Add(364 * 24 * time.Hour)
	if got := SessionExpiry(created, late, ttl, maxAge); !got.Equal(created.Add(maxAge)) {
		t.Errorf("capped at created + max age: %v", got)
	}
	if got := SessionExpiry(created, created.Add(1500*time.Nanosecond), ttl, maxAge); got.Nanosecond()%1000 != 0 {
		t.Errorf("not truncated to µs: %v", got)
	}
}

func TestClientAddrKey(t *testing.T) {
	cases := map[string]string{
		"203.0.113.5":                    "203.0.113.5",
		"::ffff:203.0.113.5":             "203.0.113.5",
		"2001:db8:1:2:3:4:5:6":           "2001:db8:1:2::/64",
		"2001:db8:1:2:ffff:ffff:ffff:ff": "2001:db8:1:2::/64",
	}
	for in, want := range cases {
		if got := ClientAddrKey(netip.MustParseAddr(in)); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}

func TestThrottleKeys(t *testing.T) {
	k := ThrottleKeys("budi@example.org", "203.0.113.5")
	if !regexp.MustCompile(`^idip:[0-9a-f]{64}:203\.0\.113\.5$`).MatchString(k[ThrottleIdentifierIP]) ||
		!regexp.MustCompile(`^id:[0-9a-f]{64}$`).MatchString(k[ThrottleIdentifier]) ||
		k[ThrottleIP] != "ip:203.0.113.5" {
		t.Errorf("keys: %v", k)
	}
	if ThrottleIdentifierInput("  0812ab ", Identifier{}, ErrInvalidIdentifier) != "0812ab" {
		t.Error("raw input must be trimmed")
	}
}
