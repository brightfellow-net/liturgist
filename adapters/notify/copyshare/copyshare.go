// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

// Package copyshare is the community Notifier: nothing is sent
// automatically; people copy and share messages themselves (04 §7).
// Composing the messages to copy arrives with publishing (step 5).
package copyshare

import (
	"context"

	"github.com/brightfellow-net/liturgist/app"
)

// Notifier composes no messages.
type Notifier struct{}

var _ app.Notifier = Notifier{}

// Compose returns an empty list.
func (Notifier) Compose(context.Context, app.NotifyEvent) ([]app.Message, error) {
	return []app.Message{}, nil
}
