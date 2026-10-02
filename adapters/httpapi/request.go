// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"net/netip"
	"sync"

	"github.com/brightfellow-net/liturgist/domain"
)

// RequestInfo is filled by the middleware and read by operations and the access log.
type RequestInfo struct {
	ClientAddr         string // domain.ClientAddrKey of the client
	UserAgent          string
	PresentedTokenHash string          // hash of the session cookie sent, valid or not
	Session            *domain.Session // nil when not logged in
	UserID             domain.UserID   // for the access log
	RequestID          string
}

type ctxKey int

const requestInfoKey ctxKey = iota

// RequestInfoFrom returns the request's info; never nil inside the middleware chain.
func RequestInfoFrom(ctx context.Context) *RequestInfo {
	if ri, ok := ctx.Value(requestInfoKey).(*RequestInfo); ok {
		return ri
	}
	return &RequestInfo{}
}

// RequestInfoMiddleware determines the client address (03 §5) and stores RequestInfo.
// requestID returns the request ID set by an earlier middleware.
func RequestInfoMiddleware(trusted []netip.Prefix, header string, requestID func(context.Context) string, log *slog.Logger) func(http.Handler) http.Handler {
	var once sync.Once
	warn := func() {
		once.Do(func() {
			log.Warn("Requests come through a proxy, but LITURGIST_TRUSTED_PROXIES is not set; all clients share one IP for login throttling.")
		})
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			info := &RequestInfo{
				ClientAddr: domain.ClientAddrKey(ClientIP(r, trusted, header, warn)),
				UserAgent:  r.UserAgent(),
				RequestID:  requestID(r.Context()),
			}
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestInfoKey, info)))
		})
	}
}
