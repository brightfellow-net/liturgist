// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package copyshare_test

import (
	"context"
	"testing"

	"github.com/brightfellow-net/liturgist/adapters/notify/copyshare"
	"github.com/brightfellow-net/liturgist/app"
)

func TestComposeIsEmpty(t *testing.T) {
	msgs, err := copyshare.Notifier{}.Compose(context.Background(), app.NotifyEvent{})
	if err != nil || msgs == nil || len(msgs) != 0 {
		t.Errorf("Compose: %v %v", msgs, err)
	}
}
