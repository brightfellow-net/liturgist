// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"net/netip"
	"strings"
	"time"
)

// ThrottleKind is one of the three login counters (03 §5).
type ThrottleKind string

// Throttle counters.
const (
	ThrottleIdentifierIP ThrottleKind = "idip"
	ThrottleIdentifier   ThrottleKind = "id"
	ThrottleIP           ThrottleKind = "ip"
)

// ThrottleRule is a counter's limit, window and lock duration.
type ThrottleRule struct {
	Kind   ThrottleKind
	Limit  int
	Window time.Duration
	Lock   time.Duration
}

// ThrottleRules are the approved limits (P-18).
var ThrottleRules = map[ThrottleKind]ThrottleRule{
	ThrottleIdentifierIP: {ThrottleIdentifierIP, 5, 15 * time.Minute, 15 * time.Minute},
	ThrottleIdentifier:   {ThrottleIdentifier, 50, time.Hour, time.Hour},
	ThrottleIP:           {ThrottleIP, 100, 15 * time.Minute, 15 * time.Minute},
}

// ThrottleKeys returns the three counter keys for one login attempt (schema:
// auth_throttle key encodings). identifier is the normalised identifier, or the
// trimmed raw input when it couldn't be parsed.
func ThrottleKeys(identifier, addrKey string) map[ThrottleKind]string {
	h := ThrottleIdentifierHash(identifier)
	return map[ThrottleKind]string{
		ThrottleIdentifierIP: "idip:" + h + ":" + addrKey,
		ThrottleIdentifier:   "id:" + h,
		ThrottleIP:           "ip:" + addrKey,
	}
}

// ClientAddrKey is IPv4 as is, IPv6 as its /64 prefix (schema: auth_throttle).
func ClientAddrKey(a netip.Addr) string {
	a = a.Unmap()
	if a.Is4() {
		return a.String()
	}
	p, _ := a.Prefix(64)
	return p.String()
}

// ThrottleIdentifierInput returns what the identifier counters are keyed by.
func ThrottleIdentifierInput(raw string, parsed Identifier, err error) string {
	if err != nil {
		return strings.TrimSpace(raw)
	}
	return parsed.Value
}

// ThrottleIdentifierHash is the hex SHA-256 used in identifier counter keys.
func ThrottleIdentifierHash(identifier string) string {
	sum := sha256.Sum256([]byte(identifier))
	return hex.EncodeToString(sum[:])
}
