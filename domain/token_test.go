// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package domain

import (
	"regexp"
	"testing"
)

var hexHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

func TestNewToken(t *testing.T) {
	seen := map[string]bool{}
	for range 1000 {
		tok, h := NewToken()
		if len(tok) != 43 || !hexHash.MatchString(h) || HashToken(tok) != h || seen[tok] {
			t.Fatalf("bad token %q / %q", tok, h)
		}
		seen[tok] = true
	}
}
