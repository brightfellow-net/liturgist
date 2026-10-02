// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package sysclock

import (
	"testing"
	"time"
)

// TC-P-009
func TestNowIsUTCMicroseconds(t *testing.T) {
	for range 100 {
		n := Clock{}.Now()
		if n.Location() != time.UTC || n.Nanosecond()%1000 != 0 {
			t.Fatalf("got %v", n)
		}
	}
}
