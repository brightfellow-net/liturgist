# 01 — Foundation (Implementation)

> **Document type: Implementation.** Step 1 of [SPEC.md §10](../SPEC.md#10-suggested-build-order).
> Status: **Draft**. Items marked **[P-xx]** are proposals awaiting approval ([index](README.md#3-proposed-decisions)).

## 1. Scope

Repository layout, toolchain, build, configuration, command line, the `server` builder, HTTP basics (routing, static files, security headers), logging, the error format, and CI. Persistence is in [02](02-persistence.md); auth in [03](03-identity-auth.md); tenancy and ports in [04](04-tenancy-extensions.md); the web app in [05](05-web-shell.md).

## 2. Toolchain

| Item | Value |
|---|---|
| Go | 1.27; `go.mod` declares `go 1.27` **[P-01]** |
| Module path | `github.com/brightfellow-net/liturgist` |
| Node.js / pnpm | Node 24 LTS, pnpm 11 (pinned with `packageManager` in the root `package.json`) |
| Linter | golangci-lint v2, pinned in CI **[P-01]** |
| Containers for tests | Docker (for testcontainers, see [02 §6](02-persistence.md#6-testing-strategy)) |

**License header [P-02]** at the top of every `.go`, `.ts`, `.tsx` and `.sql` file:

```
// Copyright 2026 Brightfellow contributors
// SPDX-License-Identifier: Apache-2.0
```

(`--` instead of `//` in SQL files.) A lint step fails if a source file lacks it.

## 3. Repository layout

```
cmd/liturgist/            community main: CLI and wiring (imports server/ only)
domain/                   entities, value objects, rules — stdlib only
app/                      use cases and ports (interfaces) — imports domain/ + stdlib
adapters/
  sqlstore/               shared SQL adapter + dialects (02)
  httpapi/                Huma operations, middleware, TenantResolver interface (04)
  tenancy/                community TenantResolver + URLBuilder (04)
  authpassword/           password AuthProvider (03)
  entitlements/unlimited/ community Entitlements stub (04)
  eventbus/memory/        in-memory EventBus (04)
  storage/localfs/        local-folder Storage (04)
  notify/copyshare/       community Notifier placeholder (04)
server/                   builder: Config, Option, New(); wires adapters to app
migrations/sqlite/        *.sql, embedded
migrations/postgres/      *.sql, embedded (same version numbers)
internal/                 private helpers only (env parsing, CLI prompts)
web/                      React SPA (05); web/embed.go embeds web/dist
packages/api-client/      generated OpenAPI spec + TypeScript types and client
packages/i18n/            message files shared by Go and web: en.json (source, reference) and id.json
docs/                     SPEC, PILOT, impl/, reference/
```

**Import rules** (enforced with `depguard` in golangci-lint):

| Package | May import | Must not import |
|---|---|---|
| `domain` | stdlib | anything else in this module; third-party |
| `app` | `domain`, stdlib | `adapters/...`, `server`, `net/http`, `database/sql`, third-party |
| `adapters/...` | `app`, `domain`, third-party | `server`, `cmd/...`, any other adapter package (adapters meet only through `app` ports, wired in `server`) |
| `server` | everything above | `cmd/...` |
| `cmd/liturgist` | `server`, `internal/...` | `app`, `domain`, `adapters` directly |

## 4. Build and generated files

| Make target | Does |
|---|---|
| `make web` | `pnpm install --frozen-lockfile && pnpm --filter web build` → `web/dist/` |
| `make gen` | `go run ./cmd/liturgist openapi > packages/api-client/openapi.json`, then `openapi-typescript` → `packages/api-client/src/schema.d.ts` |
| `make build` | `make gen`, `make web`, then `go build -o bin/liturgist ./cmd/liturgist` |
| `make test` | `go test ./...` (SQLite only) and `pnpm -r test` |
| `make test-pg` | `LITURGIST_TEST_POSTGRES=1 go test ./...` |
| `make lint` | golangci-lint, license-header check, `pnpm -r lint` |
| `make dev` | Go server on `:8080` and Vite dev server on `:5173` (Vite proxies `/api` to Go) |

- **Generated files are committed [P-07]:** `packages/api-client/openapi.json` and `packages/api-client/src/schema.d.ts`. CI runs `make gen` and fails if `git diff --exit-code` is non-empty.
- **Embedding the frontend [P-08]:** `web/embed.go` (package `web`) has `//go:embed all:dist` exporting `web.Dist fs.FS`. `web/dist/.keep` is committed; everything else in `web/dist/` is git-ignored. If `index.html` is missing, the server answers SPA routes with a plain HTML page "Frontend not built — run `make web`" (status 503).
- Version information is injected with `-ldflags "-X github.com/brightfellow-net/liturgist/server.Version=…"`; default `dev`.

## 5. Configuration

Environment variables only in step 1 **[P-03]**. All are optional.

| Variable | Default | Meaning | Validation |
|---|---|---|---|
| `LITURGIST_DATA_DIR` | `./data` | Folder for the SQLite file and later `files/`, `backups/` | Created if missing (mode 0700); must be writable |
| `LITURGIST_LISTEN` | `:8080` | HTTP listen address | `host:port` |
| `LITURGIST_BASE_URL` | `http://localhost:8080` | Public base URL for links, cookie security and the `Origin` check | Absolute `http`/`https` URL without path, query or fragment |
| `LITURGIST_DB_DRIVER` | `sqlite` | `sqlite` or `postgres` | One of the two |
| `LITURGIST_DB_URL` | — | PostgreSQL connection URL | Required when driver is `postgres`; ignored for `sqlite` |
| `LITURGIST_AUTO_MIGRATE` | `true` | Run migrations on `serve` start | `true`/`false` |
| `LITURGIST_SESSION_TTL` | `2160h` (90 days) | Session lifetime, extended on use | Go duration, ≥ `1h` |
| `LITURGIST_SESSION_MAX_AGE` | `8760h` (1 year) | Absolute session lifetime regardless of use | Go duration, ≥ `LITURGIST_SESSION_TTL` |
| `LITURGIST_TRUSTED_PROXIES` | empty | Addresses or ranges of reverse proxies allowed to report the client IP (e.g. `127.0.0.1/32,10.0.0.0/8`) | Comma-separated IPs or CIDRs |
| `LITURGIST_CLIENT_IP_HEADER` | empty | Single header with the client IP from a trusted proxy (e.g. `CF-Connecting-IP`); empty = use `X-Forwarded-For` | Header name |
| `LITURGIST_LOG_FORMAT` | `text` | `text` or `json` | One of the two |
| `LITURGIST_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` | One of the four |

- Parsed once in `cmd/liturgist` (via `internal/envconfig`) into `server.Config`. The SaaS builds `server.Config` itself.
- Invalid configuration: print every problem to stderr in one message, exit code 2.
- `cmd/liturgist` imports `time/tzdata` so time zones work on Windows and minimal container images.

## 6. Command line

Commands in step 1 **[P-04]**:

| Command | Behaviour | Exit codes |
|---|---|---|
| `liturgist serve [--allow-newer-schema]` | Load config, open the database, migrate if `LITURGIST_AUTO_MIGRATE` (with the pre-upgrade copy, [02 §5](02-persistence.md#5-migrations)), start HTTP; on SIGINT/SIGTERM stop accepting requests and finish in-flight ones within 15 s | 0 normal stop; 1 runtime error; 2 config error; 3 database newer than binary; 7 more than one church in the database |
| `liturgist migrate` | Apply pending migrations and print the resulting version | 0 / 1 / 3 |
| `liturgist migrate status [--allow-newer-schema]` | Print current and latest version | 0 / 1 / 3 |
| `liturgist setup` | Create the church and first admin (flags in [03 §10](03-identity-auth.md#10-first-time-setup)) | 0; 1 error; 4 already set up |
| `liturgist setup-link` | Print a fresh 24-hour setup link in a framed block ([03 §10](03-identity-auth.md#10-first-time-setup)) | 0; 1 error; 4 already set up |
| `liturgist user reset-password <identifier>` | Print a reset link ([03 §9](03-identity-auth.md#9-password-reset)); warn on stderr if `LITURGIST_BASE_URL` is not set | 0; 1 error; 5 no such user |
| `liturgist user list` | Print one line per user: name, email, phone, roles in the church | 0; 1 error |
| `liturgist member grant-admin <identifier>` | Emergency recovery: give that member the ready-made Church admin role (recreated with its default scopes if it was deleted); log `grant_admin_cli` with the user ID | 0; 1 error; 5 no such user; 6 not a member |
| `liturgist openapi` | Print the OpenAPI 3.1 document as JSON to stdout, without opening a database | 0 |
| `liturgist version` | Print version, commit, build date | 0 |

## 7. The `server` builder

```go
package server

type Config struct {
    DataDir     string
    Listen      string
    BaseURL     *url.URL
    DBDriver    string // "sqlite" | "postgres"
    DBURL       string
    DBMaxConns  int // PostgreSQL pool size; 0 = default 10
    DBMaxReaders int // SQLite reader pool size; 0 = default 4
    AutoMigrate bool
    SessionTTL  time.Duration
    SessionMaxAge time.Duration
    TrustedProxies []netip.Prefix
    ClientIPHeader string
    Logger      *slog.Logger
}

type Option func(*options)

func WithEntitlements(e app.Entitlements) Option
func WithNotifier(n app.Notifier) Option
func WithStorage(s app.Storage) Option
func WithEventBus(b app.EventBus) Option
func WithTenantResolver(r httpapi.TenantResolver) Option
func WithURLBuilder(u app.URLBuilder) Option
func WithRoutes(fn func(api huma.API, deps Deps)) Option // SaaS adds operations; never replaces community ones

func New(ctx context.Context, cfg Config, opts ...Option) (*Server, error)
func (s *Server) Handler() http.Handler
func (s *Server) Run(ctx context.Context) error
func (s *Server) Close() error
```

- Every option has a community default; `New` with no options gives the community edition.
- `WithRoutes` panics at startup if it registers an operation ID or method+path that already exists.

## 8. HTTP basics

| Path | Served by |
|---|---|
| `/healthz` | 200 `ok` while the process runs |
| `/readyz` | 200 when the database answers `SELECT 1` within 2 s and migrations are current; otherwise 503 with a short reason |
| `/api/v1/...` | Huma operations; OpenAPI at `/api/v1/openapi.json` |
| `/assets/...` | Hashed static files from `web/dist/assets`, `Cache-Control: public, max-age=31536000, immutable` |
| any other `GET`/`HEAD` | `web/dist/index.html` with `Cache-Control: no-cache` (SPA routing) |
| other methods outside `/api` | 405 |

- **Secret tokens only in URL fragments [P-05]:** invite, reset and setup links have the form `{base}/invite#t=<token>`. Fragments are never sent to the server, so tokens never reach access logs or `Referer` headers. The SPA reads the fragment and sends the token in a JSON body.
- **Security headers on every response [P-30]:**

| Header | Value |
|---|---|
| `Content-Security-Policy` | `default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'` |
| `X-Content-Type-Options` | `nosniff` |
| `Referrer-Policy` | `same-origin` |
| `Strict-Transport-Security` | `max-age=31536000` — only when `BaseURL` is `https` |

- Request bodies on `/api` are limited to 1 MiB in step 1 (larger limits per operation come with imports).
- Middleware order: recover → request ID → security headers → logging → session ([03 §4](03-identity-auth.md#4-sessions)) → CSRF ([03 §6](03-identity-auth.md#6-csrf-protection)) → tenant ([04 §2](04-tenancy-extensions.md#2-tenant-context)) → Huma.

## 9. Logging

- `log/slog` to stdout; handler chosen by `LITURGIST_LOG_FORMAT`.
- **Request ID:** accept an incoming `X-Request-ID` if it matches `^[A-Za-z0-9-]{8,64}$`, else generate a ULID; echo it in the response header; add `request_id` to every log line of that request.
- **Access log line** (level info): `method`, `path` (no query string), `status`, `duration_ms`, `request_id`, `user_id` (if logged in). Never request or response bodies, headers, cookies, identifiers (emails/phones) or names.
- The setup link is the only secret ever logged ([03 §10](03-identity-auth.md#10-first-time-setup)); it expires after 24 hours and is useless once setup is done.
- Panics: log at error level with stack trace and request ID; respond 500 `internal`.

## 10. Error format and codes

RFC 9457 problem details (`application/problem+json`, Huma's default) extended with a stable `code` field **[P-06]**. Clients translate by `code`, never by `detail`.

```json
{
  "status": 409,
  "title": "Conflict",
  "code": "already_member",
  "detail": "This person is already a member of the church.",
  "errors": []
}
```

| Code | HTTP | When |
|---|---|---|
| `validation_failed` | 422 | Body fails schema or field validation; `errors` lists fields |
| `weak_password` | 422 | Password rule broken; `reason`: `too_short`, `too_long`, `common`, `matches_identity` |
| `invalid_identifier` | 422 | Email/phone cannot be parsed or normalised |
| `unauthenticated` | 401 | No valid session |
| `invalid_credentials` | 401 | Login failed (same for unknown identifier and wrong password) |
| `forbidden` | 403 | Member lacks the needed scope ([04 §5](04-tenancy-extensions.md#5-authorization)) |
| `scope_not_held` | 403 | Granting or assigning scopes the actor doesn't hold; `scopes` lists them |
| `csrf_rejected` | 403 | CSRF check failed ([03 §6](03-identity-auth.md#6-csrf-protection)) |
| `invite_identifier_mismatch` | 403 | The invite's identifier belongs to another account than the logged-in one |
| `limit_reached` | 403 | Entitlement limit reached; `limit`: limit name, `used`, `max` |
| `not_found` | 404 | Resource missing, or caller not a member of the church |
| `invalid_token` | 400 | Invite/reset/setup token unusable; `reason`: `unknown`, `expired`, `used`, `cancelled` |
| `already_set_up` | 409 | Setup attempted when a church exists |
| `not_set_up` | 409 | Church endpoint called before setup |
| `already_member` | 409 | Invite for someone already a member |
| `invite_exists` | 409 | Pending invite for the same identifier exists |
| `identifier_taken` | 409 | Email/phone belongs to another user |
| `lockout_prevented` | 409 | Change would leave nobody holding both `roles.manage` and `members.manage` |
| `role_name_taken` | 409 | Another role in the church has the same name |
| `reset_not_allowed` | 409 | Admin reset for a user who belongs to another church |
| `too_many_attempts` | 429 | Login throttled; `Retry-After` header in seconds |
| `internal` | 500 | Unexpected error; details only in the log |
| `unavailable` | 503 | Database unreachable |

## 11. CI (GitHub Actions)

One workflow on every push and pull request:

1. `go vet`, golangci-lint v2, license-header check.
2. `go test ./...` with `LITURGIST_TEST_POSTGRES=1` (Docker available on `ubuntu-latest`).
3. `make gen` + `git diff --exit-code`.
4. `pnpm install --frozen-lockfile`, `pnpm -r lint`, `pnpm -r test`, `pnpm --filter web build`.
5. Cross-compile check: `GOOS=windows GOARCH=amd64` and `GOOS=linux GOARCH=arm64` `go build ./cmd/liturgist`.

## 12. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Import `net/http`, `database/sql` or a third-party package in `domain` or `app` | Keep them in `adapters`; `depguard` enforces it | Clean architecture; the SaaS and tests swap adapters |
| Put tokens in URL paths or query strings | URL fragments (`#t=`) and JSON bodies **[P-05]** | Paths and queries end up in logs, history and `Referer` |
| Log request bodies, query strings, emails, phone numbers or names | Log method, path, status, duration, IDs | UU PDP; secrets in bodies |
| Read environment variables outside `cmd/liturgist`/`internal/envconfig` | Pass `server.Config` | The SaaS configures the server differently |
| Hand-edit `openapi.json` or `schema.d.ts` | `make gen` | CI rejects drift; the Go types are the source |
| Use `log.Printf` or `fmt.Println` for logging | `slog` with request ID | Structured, filterable logs |
| Rely on the system time-zone database | Import `time/tzdata` | Windows and `scratch` images have none |
| Let `WithRoutes` replace a community operation | Panic on duplicate operation IDs or paths | The SaaS only adds endpoints (decisions log) |

## 13. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-F-001 | `envconfig` | No variables set | `server.Config` with every default in §5 | — |
| TC-F-002 | `envconfig` | `LITURGIST_DB_DRIVER=postgres`, no `LITURGIST_DB_URL` | Error naming the missing variable; exit 2 | Several errors reported together |
| TC-F-003 | `envconfig` | `LITURGIST_BASE_URL=https://x.org/path` | Error: base URL must not have a path | Trailing slash allowed and removed |
| TC-F-004 | Request ID middleware | Incoming `X-Request-ID: abc` (too short) | New ULID generated and echoed | Valid incoming ID is kept |
| TC-F-005 | Security headers | Any request | All headers in §8; HSTS only for `https` base URL | — |
| TC-F-006 | Error mapper | `app.ErrConflict{Code:"already_member"}` | 409 problem JSON with `code` | Unknown error → 500 `internal`, no detail leaked |
| TC-F-007 | Access log | Request to `/api/v1/auth/login?x=1` with body | Log line has path without query; no body | — |
| TC-F-008 | `WithRoutes` | Register an operation with an existing operation ID | Panic at `New` | Same path, different method is allowed |

### Integration tests

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-F-001 | `serve` start-up | Empty temp data dir | `/healthz` 200; `/readyz` 200; database file created | Stop server |
| IT-F-002 | SPA fallback | Built `dist` fixture | `GET /members` returns `index.html` with `no-cache`; `GET /assets/x.js` has immutable cache | — |
| IT-F-003 | Frontend not built | Empty `dist` | `GET /` → 503 "Frontend not built" page; `/api/v1/setup/status` still works | — |
| IT-F-004 | Graceful shutdown | Slow handler in flight, then SIGTERM | In-flight request completes; new connections refused | — |
| IT-F-005 | `liturgist openapi` | No database | Valid OpenAPI 3.1 JSON; matches committed file | — |

## 14. Error handling matrix

| Error | Detection | Response | Fallback | Logging |
|---|---|---|---|---|
| Invalid configuration | Parse at start | Print all problems, exit 2 | — | stderr only |
| Data dir not writable | `os.MkdirAll` / test write at start | Exit 1 with path and OS error | — | error |
| Database unreachable at start | `Ping` with 10 s timeout | Exit 1 | — | error |
| Database unreachable while running | Query error | 503 `unavailable`; `/readyz` 503 | Retry on next request | error (rate-limited to 1/min) |
| Panic in handler | Recover middleware | 500 `internal` | — | error with stack |
| Body too large | `http.MaxBytesReader` | 413 problem `validation_failed` | — | info |
| Frontend missing | `index.html` not in `web.Dist` | 503 HTML page | API keeps working | warn once at start |

## 15. References

| Topic | Location |
|---|---|
| Build order, step 1 | [SPEC.md §10](../SPEC.md#10-suggested-build-order) |
| Package layout and server options (decision) | [SPEC.md §11](../SPEC.md#11-decisions-log) — rows "Go package layout", "The SaaS customises the API only by adding endpoints" |
| Libraries (decision) | [SPEC.md §11](../SPEC.md#11-decisions-log) — rows "Backend libraries", "Frontend libraries" |
| Self-host operations (later steps) | [SPEC.md §8.3](../SPEC.md#83-self-host-operations) |
| Persistence | [02-persistence.md](02-persistence.md) |
| Sessions and CSRF | [03-identity-auth.md §4](03-identity-auth.md#4-sessions), [§6](03-identity-auth.md#6-csrf-protection) |
| Tenant middleware | [04-tenancy-extensions.md §2](04-tenancy-extensions.md#2-tenant-context) |
