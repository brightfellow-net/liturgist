// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package ulidgen

import "testing"

func TestMonotonic(t *testing.T) {
	g := New()
	prev := ""
	for range 10000 {
		id := g.NewID()
		if len(id) != 26 || id <= prev {
			t.Fatalf("%q after %q", id, prev)
		}
		prev = id
	}
}
