// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"mime"
	"net/http"
	"net/url"
)

// CSRF applies layers 2 and 3 of 03 §6: Go's cross-origin protection with the
// base URL as a trusted origin, and JSON-only unsafe requests (with or without a body).
func CSRF(baseURL *url.URL) (func(http.Handler) http.Handler, error) {
	cop := http.NewCrossOriginProtection()
	if err := cop.AddTrustedOrigin(baseURL.Scheme + "://" + baseURL.Host); err != nil {
		return nil, err
	}
	reject := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		WriteProblem(w, problem(http.StatusForbidden, "csrf_rejected", "The request was rejected as a possible cross-site request."))
	})
	cop.SetDenyHandler(reject)
	return func(next http.Handler) http.Handler {
		return cop.Handler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if unsafeMethod(r.Method) {
				if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
					reject(w, r)
					return
				}
			}
			next.ServeHTTP(w, r)
		}))
	}, nil
}

func unsafeMethod(m string) bool {
	return m == http.MethodPost || m == http.MethodPut || m == http.MethodPatch || m == http.MethodDelete
}
