// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"testing"

	"github.com/brightfellow-net/liturgist/app"
)

// TC-609: a full disk is 507 storage_full, a busy database stays 503.
func TestStorageFullProblem(t *testing.T) {
	log := slog.New(slog.DiscardHandler)
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{fmt.Errorf("%w: %w", app.ErrStorageFull, errors.New("database or disk is full")), http.StatusInsufficientStorage, "storage_full"},
		{app.ErrStorageFull, http.StatusInsufficientStorage, "storage_full"},
		{fmt.Errorf("%w: %w", app.ErrUnavailable, context.DeadlineExceeded), http.StatusServiceUnavailable, "unavailable"},
	} {
		p, ok := MapError(context.Background(), tc.err, log).(*Problem)
		if !ok || p.Status != tc.status || p.Code != tc.code {
			t.Errorf("%v: got %+v", tc.err, p)
		}
	}
}
