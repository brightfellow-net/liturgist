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

	"github.com/brightfellow-net/liturgist/adapters/argon2pw"
	"github.com/brightfellow-net/liturgist/adapters/httpapi"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/sysclock"
	"github.com/brightfellow-net/liturgist/adapters/tenancy"
	"github.com/brightfellow-net/liturgist/adapters/ulidgen"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/internal/logging"
	"github.com/danielgtaylor/huma/v2"
)

// Option customises the server. Every option has a community default.
type Option func(*options)

type options struct {
	routes   []func(api huma.API)
	dist     fs.FS // built frontend; nil = the embedded web/dist (tests inject their own)
	resolver httpapi.TenantResolver
	urls     app.URLBuilder
	clock    app.Clock // tests only
}

// WithRoutes lets the SaaS add operations. It may only add: registering an
// operation ID or method+path that already exists panics in New.
func WithRoutes(fn func(api huma.API)) Option {
	return func(o *options) { o.routes = append(o.routes, fn) }
}

// WithTenantResolver replaces the community single-church resolver (04 §3).
// The start-up church-count check and the setup link are then skipped.
func WithTenantResolver(r httpapi.TenantResolver) Option {
	return func(o *options) { o.resolver = r }
}

// WithURLBuilder replaces the community no-prefix URL builder (04 §4).
func WithURLBuilder(b app.URLBuilder) Option { return func(o *options) { o.urls = b } }

// Server is a configured Liturgist HTTP server.
type Server struct {
	cfg     Config
	db      *sqlstore.DB
	handler http.Handler
	single  *tenancy.SingleChurch // nil when the SaaS supplies a resolver
	urls    app.URLBuilder
	uc      useCases
}

// useCases are the wired application use cases.
type useCases struct {
	auth     *app.Auth
	account  *app.Account
	setup    *app.Setup
	churches *app.Churches
}

// wire builds the use cases on db with the options' adapters.
func wire(cfg Config, db app.Tx, o *options, refresh func()) useCases {
	clock := o.clock
	if clock == nil {
		clock = sysclock.Clock{}
	}
	if o.urls == nil {
		o.urls = tenancy.NoPrefix{BaseURL: cfg.BaseURL}
	}
	ids := ulidgen.New()
	hasher := argon2pw.New(argon2pw.Default)
	auth := &app.Auth{Tx: db, Hasher: hasher, Clock: clock, SessionTTL: cfg.SessionTTL, SessionMaxAge: cfg.SessionMaxAge}
	return useCases{
		auth:     auth,
		account:  &app.Account{Tx: db, Hasher: hasher, Clock: clock, Auth: auth},
		setup:    &app.Setup{Tx: db, Hasher: hasher, Clock: clock, IDs: ids, Auth: auth, OnDone: refresh},
		churches: &app.Churches{Tx: db, Clock: clock},
	}
}

// New opens the database, checks or migrates the schema, checks the church
// count (community), and builds the HTTP handler.
func New(ctx context.Context, cfg Config, opts ...Option) (*Server, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	cfg = withDefaults(cfg)
	db, mo, err := openDB(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err := prepareSchema(ctx, cfg, db, mo); err != nil {
		_ = db.Close()
		return nil, err
	}
	s := &Server{cfg: cfg, db: db}
	refresh := func() {}
	if o.resolver == nil {
		s.single = &tenancy.SingleChurch{Tx: db}
		if err := s.single.Check(ctx); err != nil {
			_ = db.Close()
			return nil, err
		}
		o.resolver = s.single
		refresh = func() {
			if err := s.single.Refresh(context.Background()); err != nil {
				cfg.Logger.Error("tenant refresh after setup failed", "error", err)
			}
		}
	}
	s.uc = wire(cfg, db, &o, refresh)
	s.urls = o.urls
	cookies := httpapi.Cookies{Secure: cfg.BaseURL.Scheme == "https"}
	d := deps{
		auth:     httpapi.AuthDeps{Auth: s.uc.auth, Account: s.uc.account, Cookies: cookies, Clock: s.uc.auth.Clock, Log: cfg.Logger},
		church:   httpapi.ChurchDeps{Setup: s.uc.setup, Churches: s.uc.churches, Cookies: cookies, Clock: s.uc.auth.Clock, Log: cfg.Logger},
		session:  httpapi.SessionMiddleware(s.uc.auth, cookies, s.uc.auth.Clock, cfg.Logger),
		resolver: o.resolver,
	}
	if s.handler, err = newHandler(cfg, o, readyCheck(db, mo), d); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil // cfg includes the defaults
}

// IsTooManyChurches reports whether err means the database holds more than
// one church (serve exits with code 7).
func IsTooManyChurches(err error) bool { return errors.Is(err, app.ErrTooManyChurches) }

// Handler returns the server's root HTTP handler.
func (s *Server) Handler() http.Handler { return s.handler }

// Run prints the setup link if the church isn't set up yet, and serves
// until ctx is cancelled, then shuts down gracefully within 15 s.
func (s *Server) Run(ctx context.Context) error {
	for _, w := range StartupWarnings(s.cfg) {
		s.cfg.Logger.Warn(w)
	}
	if s.single != nil {
		if err := s.announceSetup(ctx); err != nil {
			return err
		}
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

// announceSetup issues a new setup token when no church exists and prints
// the link framed on Notices and once in the log (03 §10, accepted risk P-42).
func (s *Server) announceSetup(ctx context.Context) error {
	token, _, err := s.uc.setup.IssueToken(ctx)
	if errors.Is(err, app.ErrAlreadySetUp) {
		return nil
	}
	if err != nil {
		return err
	}
	link := s.setupLink(ctx, token)
	if s.cfg.Notices != nil {
		_, _ = fmt.Fprint(s.cfg.Notices, SetupLinkBlock("Liturgist is not set up yet.", link))
	}
	s.cfg.Logger.Warn("Liturgist is not set up yet; open the setup link to create your church", logging.SetupLinkKey, link)
	return nil
}

func (s *Server) setupLink(ctx context.Context, token string) string {
	return s.urls.AppURL(ctx, "/setup") + "#t=" + token
}

// SetupLinkBlock frames a setup link for stderr (03 §10).
func SetupLinkBlock(headline, link string) string {
	const rule = "================================================================\n"
	return rule + "  " + headline + "\n  Open this link to create your church (valid 24 hours):\n  " + link + "\n" + rule
}

// Close closes the database.
func (s *Server) Close() error { return s.db.Close() }

// OpenAPI returns the OpenAPI document without opening a database (liturgist openapi).
func OpenAPI(opts ...Option) ([]byte, error) {
	var o options
	for _, opt := range opts {
		opt(&o)
	}
	api := newAPI(nil, o, deps{})
	return api.OpenAPI().MarshalJSON()
}
