// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

// Option customises the server. Every option has a community default.
type Option func(*options)

type options struct {
	routes []func(api huma.API)
	dist   fs.FS // built frontend; nil = the embedded web/dist (tests inject their own)
}

// WithRoutes lets the SaaS add operations. It may only add: registering an
// operation ID or method+path that already exists panics in New.
func WithRoutes(fn func(api huma.API)) Option {
	return func(o *options) { o.routes = append(o.routes, fn) }
}

// Server is a configured Liturgist HTTP server.
type Server struct {
	cfg     Config
	handler http.Handler
}

// New builds the server. The context is reserved for start-up work (database, migrations) in later slices.
func New(_ context.Context, cfg Config, opts ...Option) (*Server, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	h, err := newHandler(cfg, o, nil)
	if err != nil {
		return nil, err
	}
	return &Server{cfg: cfg, handler: h}, nil
}

// Handler returns the server's root HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Run serves until ctx is cancelled, then shuts down gracefully within 15 s.
func (s *Server) Run(ctx context.Context) error {
	for _, w := range StartupWarnings(s.cfg) {
		s.cfg.Logger.Warn(w)
	}
	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return ctx },
	}
	errc := make(chan error, 1)
	go func() { errc <- srv.ListenAndServe() }()
	s.cfg.Logger.Info("listening", "addr", s.cfg.Listen, "version", Version)

	select {
	case err := <-errc:
		return fmt.Errorf("http server: %w", err)
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-errc; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// Close releases resources. Database handles arrive in slice 2.
func (s *Server) Close() error { return nil }

// OpenAPI returns the OpenAPI document without opening a database (liturgist openapi).
func OpenAPI(opts ...Option) ([]byte, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	api := newAPI(nil, o)
	return api.OpenAPI().MarshalJSON()
}
