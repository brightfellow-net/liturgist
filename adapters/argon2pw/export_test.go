// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package argon2pw

import "context"

func (h *Hasher) Acquire(ctx context.Context) error { return h.acquire(ctx) }
func (h *Hasher) Release()                          { h.release() }

const MaxWaiting = maxWaiting
