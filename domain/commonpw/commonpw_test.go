// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package commonpw

import (
	"slices"
	"testing"
	"unicode/utf8"
)

func TestList(t *testing.T) {
	l := list()
	if len(l) < 5000 || !slices.IsSorted(l) {
		t.Fatalf("list has %d entries, sorted=%v", len(l), slices.IsSorted(l))
	}
	for _, s := range l {
		if utf8.RuneCountInString(s) < minLength {
			t.Fatalf("short entry %q kept", s)
		}
	}
	for _, s := range []string{"1234567890", "password123", "haleluya2026", "tuhanyesus1", "pujituhan123"} {
		if !Contains(s) {
			t.Errorf("%q not found", s)
		}
	}
	if Contains("kopi susu pagi hari") {
		t.Error("unexpected match")
	}
	t.Logf("%d entries", len(l))
}
