// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package memory_test

import (
	"context"
	"sync"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/eventbus/memory"
)

func TestPublishSubscribe(t *testing.T) {
	var b memory.Bus
	ctx := context.Background()
	a, cancelA := b.Subscribe(ctx, "liturgy/1")
	other, cancelOther := b.Subscribe(ctx, "liturgy/2")
	defer cancelOther()

	payload := []byte("v1")
	if err := b.Publish(ctx, "liturgy/1", payload); err != nil {
		t.Fatal(err)
	}
	payload[1] = '2' // the published copy is unaffected
	if got := <-a; string(got) != "v1" {
		t.Errorf("got %q", got)
	}
	select {
	case m := <-other:
		t.Errorf("other topic received %q", m)
	default:
	}

	cancelA()
	cancelA() // twice is fine
	if _, ok := <-a; ok {
		t.Error("channel must be closed after cancel")
	}
	if err := b.Publish(ctx, "liturgy/1", []byte("v3")); err != nil { // no subscribers left
		t.Fatal(err)
	}
}

func TestContextEndsSubscription(t *testing.T) {
	var b memory.Bus
	ctx, cancel := context.WithCancel(context.Background())
	ch, _ := b.Subscribe(ctx, "t")
	cancel()
	if _, ok := <-ch; ok {
		t.Error("channel must be closed when the context is done")
	}
	ch, end := b.Subscribe(ctx, "t") // already done
	if _, ok := <-ch; ok {
		t.Error("subscribing with a done context must give a closed channel")
	}
	end()
}

// A slow subscriber misses messages instead of blocking Publish.
func TestFullBufferDrops(t *testing.T) {
	var b memory.Bus
	ctx := context.Background()
	ch, cancel := b.Subscribe(ctx, "t")
	defer cancel()
	for range memory.Buffer + 10 {
		if err := b.Publish(ctx, "t", []byte("x")); err != nil {
			t.Fatal(err)
		}
	}
	if len(ch) != memory.Buffer {
		t.Errorf("buffered %d", len(ch))
	}
}

// Publishing while subscriptions end must not panic (send on closed channel).
func TestConcurrentCancel(_ *testing.T) {
	var b memory.Bus
	ctx := context.Background()
	var wg sync.WaitGroup
	for range 50 {
		_, cancel := b.Subscribe(ctx, "t")
		wg.Add(2)
		go func() { defer wg.Done(); cancel() }()
		go func() { defer wg.Done(); _ = b.Publish(ctx, "t", []byte("x")) }()
	}
	wg.Wait()
}
