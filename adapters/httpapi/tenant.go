// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
)

// TenantResolver finds the church a request is for (04 §3).
type TenantResolver interface {
	Resolve(r *http.Request) (domain.ChurchID, error) // app.ErrNotSetUp if no church exists
}

// TenancyOf returns an operation's declared tenancy; undeclared means church (04 §2).
func TenancyOf(op *huma.Operation) string {
	if t, ok := op.Metadata[TenancyKey].(string); ok && t != "" {
		return t
	}
	return TenancyChurch
}

// TenantMiddleware resolves the tenant for church and optional operations
// and stores it with app.WithTenant. Register it before any operation.
func TenantMiddleware(res TenantResolver, log *slog.Logger) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		tenancy := TenancyOf(ctx.Operation())
		if tenancy == TenancyPlatform {
			next(ctx)
			return
		}
		r, _ := humachi.Unwrap(ctx)
		id, err := res.Resolve(r)
		switch {
		case err == nil:
			next(huma.WithContext(ctx, app.WithTenant(ctx.Context(), id)))
		case errors.Is(err, app.ErrNotSetUp) && tenancy == TenancyOptional:
			next(ctx)
		case errors.Is(err, app.ErrNotSetUp):
			writeHumaProblem(ctx, problem(http.StatusConflict, "not_set_up", "Liturgist is not set up yet."))
		default:
			log.Error("tenant resolver failed", "error", err, "request_id", RequestInfoFrom(ctx.Context()).RequestID)
			writeHumaProblem(ctx, problem(http.StatusServiceUnavailable, "unavailable", ""))
		}
	}
}

func writeHumaProblem(ctx huma.Context, p *Problem) {
	ctx.SetHeader("Content-Type", "application/problem+json")
	ctx.SetStatus(p.Status)
	_ = json.NewEncoder(ctx.BodyWriter()).Encode(p)
}
