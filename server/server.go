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
	"path/filepath"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/argon2pw"
	"github.com/brightfellow-net/liturgist/adapters/entitlements/unlimited"
	"github.com/brightfellow-net/liturgist/adapters/eventbus/memory"
	"github.com/brightfellow-net/liturgist/adapters/httpapi"
	"github.com/brightfellow-net/liturgist/adapters/importers/chordpro"
	"github.com/brightfellow-net/liturgist/adapters/importers/openlyrics"
	"github.com/brightfellow-net/liturgist/adapters/importers/paste"
	"github.com/brightfellow-net/liturgist/adapters/notify/copyshare"
	"github.com/brightfellow-net/liturgist/adapters/sqlstore"
	"github.com/brightfellow-net/liturgist/adapters/storage/localfs"
	"github.com/brightfellow-net/liturgist/adapters/sysclock"
	"github.com/brightfellow-net/liturgist/adapters/tenancy"
	"github.com/brightfellow-net/liturgist/adapters/ulidgen"
	"github.com/brightfellow-net/liturgist/app"
	"github.com/brightfellow-net/liturgist/domain"
	"github.com/brightfellow-net/liturgist/internal/logging"
	"github.com/danielgtaylor/huma/v2"
)

// Option customises the server. Every option has a community default.
type Option func(*options)

type options struct {
	routes       []func(api huma.API)
	dist         fs.FS // built frontend; nil = the embedded web/dist (tests inject their own)
	resolver     httpapi.TenantResolver
	entitlements app.Entitlements
	urls         app.URLBuilder
	storage      app.Storage  // unused until logo upload; wired so later steps only read it
	events       app.EventBus // unused until the liturgy editor (step 3)
	notifier     app.Notifier // unused until publishing (step 5)
	clock        app.Clock    // tests only

	bibleProviders []app.BibleTextProvider
	importers      map[domain.ImportFormat]app.Importer
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

// WithEntitlements replaces the community "everything allowed" stub (04 §8).
func WithEntitlements(e app.Entitlements) Option { return func(o *options) { o.entitlements = e } }

// WithURLBuilder replaces the community no-prefix URL builder (04 §4).
func WithURLBuilder(b app.URLBuilder) Option { return func(o *options) { o.urls = b } }

// WithStorage replaces the community local-folder storage (<DataDir>/files).
func WithStorage(s app.Storage) Option { return func(o *options) { o.storage = s } }

// WithEventBus replaces the community in-process event bus.
func WithEventBus(b app.EventBus) Option { return func(o *options) { o.events = b } }

// WithBibleTextProvider appends a provider of Bible text; readings are looked
// up in the providers in the order they were added (07 §3.1).
func WithBibleTextProvider(p app.BibleTextProvider) Option {
	return func(o *options) { o.bibleProviders = append(o.bibleProviders, p) }
}

// WithImporter adds or replaces the importer of a format (08 §7). The
// community edition registers paste, openlyrics and chordpro.
func WithImporter(format domain.ImportFormat, imp app.Importer) Option {
	return func(o *options) {
		if o.importers == nil {
			o.importers = map[domain.ImportFormat]app.Importer{}
		}
		o.importers[format] = imp
	}
}

// WithNotifier replaces the community copy-and-share notifier.
func WithNotifier(n app.Notifier) Option { return func(o *options) { o.notifier = n } }

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
	auth      *app.Auth
	account   *app.Account
	setup     *app.Setup
	churches  *app.Churches
	members   *app.Members
	roles     *app.Roles
	invites   *app.Invites
	resets    *app.Resets
	songs     *app.Songs
	readings  *app.Readings
	imports   *app.Imports
	vocab     *app.Vocabulary
	templates *app.Templates
	services  *app.Services
	seed      *app.Seed
	operator  *app.Operator
	cleanup   *app.Cleanup
}

// wire builds the use cases on db with the options' adapters.
func wire(cfg Config, db app.Tx, o *options, refresh func()) useCases {
	clock := o.clock
	if clock == nil {
		clock = sysclock.Clock{}
	}
	if o.entitlements == nil {
		o.entitlements = unlimited.Entitlements{}
	}
	if o.urls == nil {
		o.urls = tenancy.NoPrefix{BaseURL: cfg.BaseURL}
	}
	if o.storage == nil {
		o.storage = localfs.Storage{Dir: filepath.Join(cfg.DataDir, "files")}
	}
	if o.events == nil {
		o.events = &memory.Bus{}
	}
	if o.notifier == nil {
		o.notifier = copyshare.Notifier{}
	}
	ids := ulidgen.New()
	hasher := argon2pw.New(argon2pw.Default)
	auth := &app.Auth{Tx: db, Hasher: hasher, Clock: clock, SessionTTL: cfg.SessionTTL, SessionMaxAge: cfg.SessionMaxAge}
	songs := &app.Songs{Tx: db, Clock: clock, IDs: ids, Usage: app.NeverUsed{}}
	importers := map[domain.ImportFormat]app.Importer{
		domain.FormatPaste: paste.Importer{}, domain.FormatOpenLyrics: openlyrics.Importer{}, domain.FormatChordPro: chordpro.Importer{},
	}
	for format, imp := range o.importers {
		importers[format] = imp
	}
	return useCases{
		auth:     auth,
		account:  &app.Account{Tx: db, Hasher: hasher, Clock: clock, Auth: auth},
		setup:    &app.Setup{Tx: db, Hasher: hasher, Clock: clock, IDs: ids, Auth: auth, OnDone: refresh},
		churches: &app.Churches{Tx: db, Clock: clock},
		members:  &app.Members{Tx: db, Clock: clock, Entitlements: o.entitlements},
		roles:    &app.Roles{Tx: db, Clock: clock, IDs: ids},
		invites: &app.Invites{Tx: db, Hasher: hasher, Clock: clock, IDs: ids, URLs: o.urls,
			Entitlements: o.entitlements, Auth: auth},
		songs:     songs,
		imports:   &app.Imports{Tx: db, Clock: clock, IDs: ids, Songs: songs, Importers: importers},
		readings:  &app.Readings{Tx: db, Clock: clock, IDs: ids, Usage: app.NeverUsed{}, Providers: o.bibleProviders, Log: cfg.Logger},
		vocab:     &app.Vocabulary{Tx: db, Clock: clock, IDs: ids, Usage: app.NeverUsed{}},
		templates: &app.Templates{Tx: db, Clock: clock, IDs: ids},
		services:  &app.Services{Tx: db, Clock: clock, IDs: ids},
		seed:      &app.Seed{Tx: db, Clock: clock, IDs: ids},
		resets:    &app.Resets{Tx: db, Hasher: hasher, Clock: clock, IDs: ids, URLs: o.urls, Auth: auth},
		operator:  &app.Operator{Tx: db, Clock: clock, IDs: ids},
		cleanup:   &app.Cleanup{Tx: db, Clock: clock},
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
	if err := s.uc.seed.Run(ctx); err != nil { // 09 §3: defaults for churches that have none
		cfg.Logger.Error("seeding the defaults failed; the church works without them", "error", err)
	}
	s.urls = o.urls
	cookies := httpapi.Cookies{Secure: cfg.BaseURL.Scheme == "https"}
	d := deps{
		auth: httpapi.AuthDeps{Auth: s.uc.auth, Account: s.uc.account, Cookies: cookies, Clock: s.uc.auth.Clock, Log: cfg.Logger},
		church: httpapi.ChurchDeps{Setup: s.uc.setup, Churches: s.uc.churches, Members: s.uc.members, Roles: s.uc.roles,
			Invites: s.uc.invites, Resets: s.uc.resets, Cookies: cookies, Clock: s.uc.auth.Clock, Log: cfg.Logger},
		library:  httpapi.LibraryDeps{Songs: s.uc.songs, Readings: s.uc.readings, Imports: s.uc.imports, Log: cfg.Logger},
		planning: httpapi.PlanningDeps{Vocabulary: s.uc.vocab, Templates: s.uc.templates, Services: s.uc.services, Log: cfg.Logger},
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

// Run prints the setup link if the church isn't set up yet, starts the
// cleanup job, and serves until ctx is cancelled, then shuts down gracefully
// within 15 s.
func (s *Server) Run(ctx context.Context) error {
	for _, w := range StartupWarnings(s.cfg) {
		s.cfg.Logger.Warn(w)
	}
	if s.single != nil {
		if err := s.announceSetup(ctx); err != nil {
			return err
		}
	}
	go s.cleanupLoop(ctx)

	srv := &http.Server{
		Addr:              s.cfg.Listen,
		Handler:           s.handler,
		ReadHeaderTimeout: 10 * time.Second,
		// Requests keep ctx's values but not its cancellation: on SIGTERM,
		// requests in flight must finish (IT-F-004), within Shutdown's limit.
		BaseContext: func(net.Listener) context.Context { return context.WithoutCancel(ctx) },
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

// cleanupLoop runs the cleanup job one minute after start, then throttle
// rows every 10 minutes and everything else hourly (03 §11).
func (s *Server) cleanupLoop(ctx context.Context) {
	timer := time.NewTimer(time.Minute)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
	}
	s.runCleanup(ctx, true)
	tick := time.NewTicker(10 * time.Minute)
	defer tick.Stop()
	for n := 1; ; n++ {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			s.runCleanup(ctx, n%6 == 0)
		}
	}
}

func (s *Server) runCleanup(ctx context.Context, hourly bool) {
	if err := s.uc.cleanup.Throttle(ctx); err != nil && ctx.Err() == nil {
		s.cfg.Logger.Warn("cleanup of throttle counters failed", "error", err)
	}
	if !hourly {
		return
	}
	if err := s.uc.cleanup.Hourly(ctx); err != nil && ctx.Err() == nil {
		s.cfg.Logger.Warn("cleanup failed", "error", err)
	}
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
