// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"log/slog"

	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

// The helpers shared by the Register… functions. Each group of operations is
// its own function and picks up the few it needs.

// failer returns the function that turns an error from a use case into the
// problem response (MapError, with the log of the deps).
func failer(log *slog.Logger) func(context.Context, error) error {
	return func(ctx context.Context, err error) error { return MapError(ctx, err, log) }
}

// sessionOf is the session of the request (nil without one).
func sessionOf(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }

// actorOf is the user ID written to the log as "actor".
func actorOf(ctx context.Context) string { return string(sessionOf(ctx).UserID) }

// churchOp returns the builder of the operations of one tag that need a church.
func churchOp(tag string) func(id, method, path string, status int, summary string) huma.Operation {
	return func(id, method, path string, status int, summary string) huma.Operation {
		return tagged(op(id, method, path, TenancyChurch, status, summary), tag)
	}
}
