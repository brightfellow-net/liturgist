// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package memory is the community EventBus: subscribers in the same process
// (04 §7). Several server instances (SaaS) need a shared bus instead.
package memory

import (
	"bytes"
	"context"
	"sync"

	"github.com/brightfellow-net/liturgist/app"
)

// Buffer is how many messages a subscriber may fall behind. A subscriber
// whose buffer is full misses messages rather than blocking the publisher;
// listeners must be able to re-read the current state (provisional [P-27]).
const Buffer = 64

// Bus is an in-process EventBus; the zero value is ready to use.
type Bus struct {
	mu   sync.Mutex
	subs map[string]map[*sub]struct{}
}

type sub struct{ ch chan []byte }

var _ app.EventBus = (*Bus)(nil)

// Publish delivers a copy of payload to every current subscriber of topic.
func (b *Bus) Publish(_ context.Context, topic string, payload []byte) error {
	p := bytes.Clone(payload)
	b.mu.Lock()
	defer b.mu.Unlock()
	for s := range b.subs[topic] {
		select {
		case s.ch <- p:
		default: // full: this subscriber misses the message
		}
	}
	return nil
}

// Subscribe returns a channel of topic's messages. The subscription ends,
// and the channel is closed, when cancel is called or ctx is done.
// Subscribers must not modify the payloads they receive (they are shared).
func (b *Bus) Subscribe(ctx context.Context, topic string) (<-chan []byte, func()) {
	s := &sub{ch: make(chan []byte, Buffer)}
	b.mu.Lock()
	if b.subs == nil {
		b.subs = map[string]map[*sub]struct{}{}
	}
	if b.subs[topic] == nil {
		b.subs[topic] = map[*sub]struct{}{}
	}
	b.subs[topic][s] = struct{}{}
	b.mu.Unlock()

	var once sync.Once
	end := func() {
		once.Do(func() {
			b.mu.Lock()
			defer b.mu.Unlock()
			delete(b.subs[topic], s)
			if len(b.subs[topic]) == 0 {
				delete(b.subs, topic)
			}
			close(s.ch) // under the lock, so Publish never sends on a closed channel
		})
	}
	stop := context.AfterFunc(ctx, end)
	return s.ch, func() { stop(); end() }
}
