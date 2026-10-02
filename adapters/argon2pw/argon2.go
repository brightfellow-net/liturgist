// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package argon2pw is the argon2id app.PasswordHasher (03 §3).
package argon2pw

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"golang.org/x/crypto/argon2"
)

// Params are argon2id parameters. Memory is in KiB.
type Params struct {
	Memory  uint32
	Time    uint32
	Threads uint8
}

// Default is the OWASP minimum, safe on 512 MB machines (P-17).
var Default = Params{Memory: 19456, Time: 2, Threads: 1}

const (
	saltLen    = 16
	keyLen     = 32
	slots      = 2                // concurrent computations (≈ 38 MiB)
	maxWaiting = 32               // queued requests before 503
	maxWait    = 10 * time.Second // per request
)

var errMalformed = errors.New("malformed password hash")

// Hasher implements app.PasswordHasher.
type Hasher struct {
	p       Params
	slots   chan struct{}
	waiting atomic.Int32
	dummy   string
}

var _ app.PasswordHasher = (*Hasher)(nil)

// New returns a hasher. It computes one dummy hash at start for VerifyDummy.
func New(p Params) *Hasher {
	h := &Hasher{p: p, slots: make(chan struct{}, slots)}
	h.dummy = h.encode(randomBytes(saltLen), "dummy-password-for-timing")
	return h
}

// Hash returns the PHC-encoded argon2id hash of the normalised password.
func (h *Hasher) Hash(ctx context.Context, password string) (string, error) {
	if err := h.acquire(ctx); err != nil {
		return "", err
	}
	defer h.release()
	return h.encode(randomBytes(saltLen), password), nil
}

// Verify checks password against encoded; needsRehash is true after a successful
// check when encoded uses parameters other than the hasher's.
func (h *Hasher) Verify(ctx context.Context, encoded, password string) (ok, needsRehash bool, err error) {
	p, salt, want, err := decode(encoded)
	if err != nil {
		return false, false, err
	}
	if err := h.acquire(ctx); err != nil {
		return false, false, err
	}
	defer h.release()
	got := argon2.IDKey([]byte(domain.NormalizePassword(password)), salt, p.Time, p.Memory, p.Threads, uint32(len(want))) //nolint:gosec // key length is 32
	ok = subtle.ConstantTimeCompare(got, want) == 1
	return ok, ok && p != h.p, nil
}

// VerifyDummy costs the same as Verify, for logins without a matching user.
func (h *Hasher) VerifyDummy(ctx context.Context, password string) error {
	_, _, err := h.Verify(ctx, h.dummy, password)
	return err
}

// acquire waits for a computation slot: at most maxWaiting requests may wait,
// each for at most maxWait, and a cancelled request leaves the queue at once.
func (h *Hasher) acquire(ctx context.Context) error {
	if h.waiting.Add(1) > maxWaiting {
		h.waiting.Add(-1)
		return fmt.Errorf("%w: password hashing queue full", app.ErrUnavailable)
	}
	defer h.waiting.Add(-1)
	timer := time.NewTimer(maxWait)
	defer timer.Stop()
	select {
	case h.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("%w: %w", app.ErrUnavailable, ctx.Err())
	case <-timer.C:
		return fmt.Errorf("%w: password hashing busy", app.ErrUnavailable)
	}
}

func (h *Hasher) release() { <-h.slots }

func (h *Hasher) encode(salt []byte, password string) string {
	key := argon2.IDKey([]byte(domain.NormalizePassword(password)), salt, h.p.Time, h.p.Memory, h.p.Threads, keyLen)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, h.p.Memory, h.p.Time, h.p.Threads,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func decode(encoded string) (Params, []byte, []byte, error) {
	parts := strings.Split(encoded, "$") // "", "argon2id", "v=19", "m=…,t=…,p=…", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != fmt.Sprintf("v=%d", argon2.Version) {
		return Params{}, nil, nil, errMalformed
	}
	var p Params
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &p.Memory, &p.Time, &p.Threads); err != nil {
		return Params{}, nil, nil, errMalformed
	}
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	key, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil || len(key) == 0 {
		return Params{}, nil, nil, errMalformed
	}
	return p, salt, key, nil
}

func randomBytes(n int) []byte {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return b
}
