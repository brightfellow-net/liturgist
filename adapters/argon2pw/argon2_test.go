// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package argon2pw_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/argon2pw"
	"github.com/brightfellow-net/liturgist/app"
)

var fast = argon2pw.Params{Memory: 64, Time: 1, Threads: 1}

func TestDefaultParams(t *testing.T) {
	if argon2pw.Default != (argon2pw.Params{Memory: 19456, Time: 2, Threads: 1}) {
		t.Errorf("default parameters changed: %+v", argon2pw.Default)
	}
}

// TC-A-004
func TestHashVerify(t *testing.T) {
	ctx := context.Background()
	h := argon2pw.New(fast)
	enc, err := h.Hash(ctx, "café au lait 2026")
	if err != nil || !strings.HasPrefix(enc, "$argon2id$v=19$m=64,t=1,p=1$") {
		t.Fatalf("hash %q %v", enc, err)
	}
	if ok, rehash, err := h.Verify(ctx, enc, "café au lait 2026"); !ok || rehash || err != nil {
		t.Errorf("same password, other Unicode form: ok=%v rehash=%v err=%v", ok, rehash, err)
	}
	if ok, _, _ := h.Verify(ctx, enc, "cafe au lait 2026"); ok {
		t.Error("wrong password accepted")
	}
	if ok, _, _ := h.Verify(ctx, enc, "café au lait 2026 "); ok {
		t.Error("trailing space must matter")
	}

	newer := argon2pw.New(argon2pw.Params{Memory: 128, Time: 1, Threads: 1})
	if ok, rehash, _ := newer.Verify(ctx, enc, "café au lait 2026"); !ok || !rehash {
		t.Errorf("old parameters: ok=%v rehash=%v", ok, rehash)
	}
	for _, bad := range []string{"", "$argon2i$v=19$m=64,t=1,p=1$YQ$YQ", "$argon2id$v=19$m=x$YQ$YQ", "$argon2id$v=19$m=64,t=1,p=1$!!$YQ"} {
		if _, _, err := h.Verify(ctx, bad, "x"); err == nil {
			t.Errorf("malformed %q accepted", bad)
		}
	}
	if err := h.VerifyDummy(ctx, "anything"); err != nil {
		t.Errorf("dummy: %v", err)
	}
}

// IT-A-015 (unit part)
func TestQueue(t *testing.T) {
	ctx := context.Background()
	h := argon2pw.New(fast)
	// Hold both slots.
	for range 2 {
		if err := h.Acquire(ctx); err != nil {
			t.Fatal(err)
		}
	}
	waitCtx, cancelWaiters := context.WithCancel(ctx)
	var wg sync.WaitGroup
	for range argon2pw.MaxWaiting {
		wg.Add(1)
		go func() { defer wg.Done(); _ = h.Acquire(waitCtx) }()
	}
	time.Sleep(100 * time.Millisecond) // let the waiters queue up

	start := time.Now()
	if err := h.Acquire(ctx); !errors.Is(err, app.ErrUnavailable) || time.Since(start) > 50*time.Millisecond {
		t.Errorf("33rd request must fail at once: %v after %v", err, time.Since(start))
	}

	start = time.Now()
	cancelWaiters()
	wg.Wait()
	if time.Since(start) > time.Second {
		t.Errorf("cancelled waiters did not leave promptly")
	}
	h.Release()
	h.Release()
	if err := h.Acquire(ctx); err != nil {
		t.Errorf("slot free again: %v", err)
	}
	h.Release()
}
