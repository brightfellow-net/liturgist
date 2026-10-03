// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0

package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/brightfellow-net/liturgist/adapters/httpapi"
	"github.com/brightfellow-net/liturgist/web"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
)

// deps carries what the API needs at request time; nil fields only when the
// OpenAPI document is built without a database.
type deps struct {
	auth     httpapi.AuthDeps
	church   httpapi.ChurchDeps
	library  httpapi.LibraryDeps
	planning httpapi.PlanningDeps
	session  func(http.Handler) http.Handler // nil: no session middleware (OpenAPI only)
	resolver httpapi.TenantResolver          // nil: no tenant middleware (OpenAPI only)
}

func newHandler(cfg Config, o options, ready func(context.Context) error, d deps) (http.Handler, error) {
	r := chi.NewRouter()
	r.Use(recoverer(cfg.Logger), requestID, allowedHosts(cfg), securityHeaders(cfg),
		httpapi.RequestInfoMiddleware(cfg.TrustedProxies, cfg.ClientIPHeader, RequestIDFrom, cfg.Logger),
		accessLog(cfg.Logger), noOptions)

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("ok")) })
	r.Get("/readyz", func(w http.ResponseWriter, req *http.Request) {
		if ready != nil {
			ctx, cancel := context.WithTimeout(req.Context(), 2*time.Second)
			defer cancel()
			if err := ready(ctx); err != nil {
				http.Error(w, err.Error(), http.StatusServiceUnavailable)
				return
			}
		}
		_, _ = w.Write([]byte("ok"))
	})

	apiRouter := chi.NewRouter()
	csrf, err := httpapi.CSRF(cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	if d.session != nil {
		apiRouter.Use(d.session)
	}
	apiRouter.Use(csrf)
	apiRouter.NotFound(func(w http.ResponseWriter, _ *http.Request) {
		httpapi.WriteProblem(w, &httpapi.Problem{Status: http.StatusNotFound, Title: "Not Found", Code: "not_found"})
	})
	newAPI(apiRouter, o, d)
	r.Mount("/api/v1", apiRouter)

	spaApp, err := newSPA(o.dist)
	if err != nil {
		return nil, err
	}
	r.Handle("/assets/*", spaApp.assets())
	r.NotFound(spaApp.index)
	r.MethodNotAllowed(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
	})
	return r, nil
}

// newAPI registers all operations. router may be nil (OpenAPI export only).
func newAPI(router chi.Router, o options, d deps) huma.API {
	cfg := huma.DefaultConfig("Liturgist API", Version)
	cfg.Servers = []*huma.Server{{URL: "/api/v1"}}
	cfg.OpenAPIPath = "/openapi" // served as /api/v1/openapi.json
	cfg.DocsPath = ""            // no docs UI: it loads scripts from a CDN, blocked by our CSP
	if router == nil {
		router = chi.NewRouter()
	}
	api := &guardedAPI{API: humachi.New(router, cfg), seen: map[string]bool{}}
	if d.resolver != nil {
		// Before any operation: Huma captures the middleware stack at registration.
		api.UseMiddleware(httpapi.TenantMiddleware(d.resolver, d.auth.Log))
	}
	httpapi.RegisterAuth(api, d.auth)
	httpapi.RegisterChurch(api, d.church)
	httpapi.RegisterLibrary(api, d.library)
	httpapi.RegisterPlanning(api, d.planning)
	for _, fn := range o.routes {
		fn(api)
	}
	return api
}

// guardedAPI panics when an operation ID or method+path is registered twice,
// so WithRoutes can only add operations (decisions log: SaaS adds endpoints only).
type guardedAPI struct {
	huma.API
	mu   sync.Mutex
	seen map[string]bool
}

func (g *guardedAPI) Adapter() huma.Adapter { return guardedAdapter{g.API.Adapter(), g} }

type guardedAdapter struct {
	huma.Adapter
	g *guardedAPI
}

func (a guardedAdapter) Handle(op *huma.Operation, handler func(huma.Context)) {
	a.g.mu.Lock()
	for _, k := range []string{"id:" + op.OperationID, "route:" + op.Method + " " + op.Path} {
		if a.g.seen[k] {
			a.g.mu.Unlock()
			panic(fmt.Sprintf("duplicate operation %s", k))
		}
		a.g.seen[k] = true
	}
	a.g.mu.Unlock()
	a.Adapter.Handle(op, handler)
}

// --- middleware ---

type ctxKey int

const requestIDKey ctxKey = iota

var validRequestID = regexp.MustCompile(`^[A-Za-z0-9-]{8,64}$`)

func requestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if !validRequestID.MatchString(id) {
			id = newRequestID()
		}
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), requestIDKey, id)))
	})
}

func newRequestID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return hex.EncodeToString(b[:])
}

// RequestIDFrom returns the request ID set by the middleware.
func RequestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey).(string)
	return id
}

func allowedHosts(cfg Config) func(http.Handler) http.Handler {
	allowed := map[string]bool{strings.ToLower(cfg.BaseURL.Hostname()): true}
	for _, h := range cfg.ExtraHosts {
		allowed[strings.ToLower(h)] = true
	}
	if allowed["localhost"] {
		allowed["127.0.0.1"], allowed["::1"] = true, true
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/healthz" || r.URL.Path == "/readyz" || allowed[hostOnly(r.Host)] {
				next.ServeHTTP(w, r)
				return
			}
			w.WriteHeader(http.StatusMisdirectedRequest)
		})
	}
}

func hostOnly(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return strings.ToLower(h)
	}
	return strings.ToLower(strings.Trim(hostport, "[]"))
}

func securityHeaders(cfg Config) func(http.Handler) http.Handler {
	hsts := cfg.BaseURL.Scheme == "https"
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := w.Header()
			h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'")
			h.Set("X-Content-Type-Options", "nosniff")
			h.Set("Referrer-Policy", "same-origin")
			if hsts {
				h.Set("Strict-Transport-Security", "max-age=31536000")
			}
			next.ServeHTTP(w, r)
		})
	}
}

func noOptions(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (s *statusWriter) WriteHeader(code int)        { s.status = code; s.ResponseWriter.WriteHeader(code) }
func (s *statusWriter) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func accessLog(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(sw, r)
			attrs := []any{
				"method", r.Method,
				"path", r.URL.Path, // never the query string
				"status", sw.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"request_id", RequestIDFrom(r.Context()),
			}
			if uid := httpapi.RequestInfoFrom(r.Context()).UserID; uid != "" {
				attrs = append(attrs, "user_id", string(uid))
			}
			log.Info("request", attrs...)
		})
	}
}

func recoverer(log *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if v := recover(); v != nil {
					if v == http.ErrAbortHandler { //nolint:errorlint // sentinel compared as panic value
						panic(v)
					}
					log.Error("panic", "error", fmt.Sprint(v), "stack", string(debug.Stack()),
						"request_id", RequestIDFrom(r.Context()))
					httpapi.WriteProblem(w, &httpapi.Problem{Status: http.StatusInternalServerError, Title: "Internal Server Error", Code: "internal"})
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// --- single-page app ---

type spa struct {
	dist  fs.FS
	built bool
}

func newSPA(dist fs.FS) (*spa, error) {
	if dist == nil {
		sub, err := fs.Sub(web.Dist, "dist")
		if err != nil {
			return nil, err
		}
		dist = sub
	}
	_, statErr := fs.Stat(dist, "index.html")
	if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
		return nil, statErr
	}
	return &spa{dist: dist, built: statErr == nil}, nil
}

func (s *spa) assets() http.Handler {
	files := http.FileServerFS(s.dist)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		files.ServeHTTP(w, r)
	})
}

const notBuiltPage = `<!doctype html><meta charset="utf-8"><title>Liturgist</title>
<p>Frontend not built — run <code>make web</code>.</p>`

func (s *spa) index(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Cache-Control", "no-cache")
	if !s.built {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(notBuiltPage))
		return
	}
	http.ServeFileFS(w, r, s.dist, "index.html")
}
