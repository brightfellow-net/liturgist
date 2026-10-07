// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"io"
	"net/http"

	"github.com/brightfellow-net/liturgist/domain"
	"github.com/danielgtaylor/huma/v2"
)

// logoBodyLimit is the request limit of the upload: the 2 MiB image as
// base64 (four characters for three bytes) plus the JSON around it.
const logoBodyLimit = 3 << 20

// registerLogo registers the church logo operations (15 §2, L-4).
func registerLogo(api huma.API, d ChurchDeps) {
	fail := func(ctx context.Context, err error) error { return MapError(ctx, err, d.Log) }
	sess := func(ctx context.Context) *domain.Session { return RequestInfoFrom(ctx).Session }

	// JSON with base64, because unsafe requests must be JSON (03 §6).
	set := tagged(op("setChurchLogo", http.MethodPut, "/church/logo", TenancyChurch, http.StatusOK, "Set the church logo (church.settings)"), "church")
	set.MaxBodyBytes = logoBodyLimit
	huma.Register(api, set,
		func(ctx context.Context, in *struct {
			Body struct {
				Image []byte `json:"image" doc:"A PNG, JPEG or WebP file, at most 2 MB, as base64"`
			}
		}) (*churchOutput, error) {
			res, err := d.Logos.Set(ctx, sess(ctx), in.Body.Image)
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &churchOutput{Body: churchView(res)}, nil
		})

	huma.Register(api, tagged(op("removeChurchLogo", http.MethodDelete, "/church/logo", TenancyChurch, http.StatusOK, "Remove the church logo (church.settings)"), "church"),
		func(ctx context.Context, _ *struct{}) (*churchOutput, error) {
			res, err := d.Logos.Remove(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			return &churchOutput{Body: churchView(res)}, nil
		})

	huma.Register(api, tagged(op("getChurchLogo", http.MethodGet, "/church/logo", TenancyChurch, http.StatusOK, "The church logo, a PNG"), "church"),
		func(ctx context.Context, in *struct {
			Version     string `query:"v" maxLength:"64" doc:"The version from logo_url; with the current one the file is cached for a year"`
			IfNoneMatch string `header:"If-None-Match"`
		}) (*huma.StreamResponse, error) {
			f, err := d.Logos.Open(ctx, sess(ctx))
			if err != nil {
				return nil, fail(ctx, err)
			}
			etag := `"` + f.Version + `"`
			cache := "private, no-cache"
			if in.Version == f.Version {
				cache = "private, max-age=31536000, immutable"
			}
			return &huma.StreamResponse{Body: func(hctx huma.Context) {
				defer func() { _ = f.Close() }()
				hctx.SetHeader("ETag", etag)
				hctx.SetHeader("Cache-Control", cache)
				if in.IfNoneMatch == etag {
					hctx.SetStatus(http.StatusNotModified)
					return
				}
				hctx.SetHeader("Content-Type", "image/png")
				hctx.SetStatus(http.StatusOK)
				_, _ = io.Copy(hctx.BodyWriter(), f)
			}}, nil
		})
}
