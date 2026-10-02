// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package ulidgen is the production app.IDGenerator.
package ulidgen

import (
	"crypto/rand"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Generator returns monotonic ULIDs.
type Generator struct {
	mu      sync.Mutex
	entropy *ulid.MonotonicEntropy
}

// New returns a generator.
func New() *Generator { return &Generator{entropy: ulid.Monotonic(rand.Reader, 0)} }

// NewID returns a ULID that sorts after every ID previously returned by this generator.
func (g *Generator) NewID() string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return ulid.MustNew(ulid.Timestamp(time.Now()), g.entropy).String()
}
