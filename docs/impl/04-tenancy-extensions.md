# 04 — Tenancy, Authorization and Extension Points (Implementation)

> **Document type: Implementation.** Step 1 of [SPEC.md §10](../SPEC.md#10-suggested-build-order).
> Status: **Approved** 2026-10-02. Items marked **[P-xx]** are decisions listed in the [index](README.md#4-proposed-decisions).

## 1. Scope

The tenant context, the `TenantResolver` and `URLBuilder` ports, authorization (actor, 404 vs 403, allowed actions), the step-1 API for church and members, the extension-point interfaces, and the entitlement stub.

## 2. Tenant context

- `app.Tenant{ChurchID domain.ChurchID}` is stored in the request context by the tenant middleware ([01 §8](01-foundation.md#8-http-basics) middleware order).
- **Every operation declares its tenancy** when it is registered, as Huma operation metadata `tenancy`:

| `tenancy` | Middleware behaviour | Step-1 operations |
|---|---|---|
| `church` (**default** when not declared) | Resolve the tenant; failure → 409 `not_set_up` or 503 `unavailable` | Everything not listed below |
| `optional` | Try to resolve; continue without a tenant if there is none | `GET /api/v1/me`, the three invite operations below |
| `platform` | Never resolve | `/api/v1/setup*`, `/api/v1/auth/*` (including `/auth/reset/inspect`), `PATCH /api/v1/me`, `/api/v1/me/password`, `/api/v1/me/sessions/end-others`, `/api/v1/translations` |

- Operations added with `server.WithRoutes` follow the same rule: undeclared means `church`. Declaring `platform` or `optional` is a deliberate, reviewed choice. A unit test lists every registered operation with its declared tenancy and fails if the list changes without updating the test's expected table.
- **Invite operations** (`/api/v1/invites/inspect`, `/accept`, `/accept-existing`) are `optional` and take the church from the invite:
  - Community: the resolver always returns the install's only church, which is always the invite's church.
  - SaaS: invite links carry the church slug (built by the SaaS `URLBuilder`). If a tenant is resolved and differs from the invite's church → 404 `not_found`. If the slug is unknown → 404 `not_found`.
- `GET /api/v1/me` with no tenant (not set up), or for a user who is not a member of the tenant church, returns `church` and `membership` as `null`.
- Use cases read the tenant with `app.TenantFrom(ctx)`; it returns an error if absent. Use cases pass `Tenant.ChurchID` to `Store.ForChurch` ([02 §2](02-persistence.md#2-ports-in-app)).
- **Enforcing scoped access:** every `ChurchStore` repository method is covered by the two-church contract test IT-P-007 ([02 §10](02-persistence.md#10-test-case-specifications)), and composite foreign keys reject cross-church references. When `liturgist-saas` adds row-level-security policies, they must deny all rows when `liturgist.church_id` is unset (fail closed).

## 3. `TenantResolver`

Defined in `adapters/httpapi` (it depends on `*http.Request`):

```go
type TenantResolver interface {
    Resolve(r *http.Request) (domain.ChurchID, error) // app.ErrNotSetUp if no church exists
}
```

**Community implementation [P-25]** (`adapters/tenancy.SingleChurch`): returns the ID of the only row in `churches`, cached in memory after the first successful lookup. `Refresh()` is called by the setup use case after creating the church. It never reads the church from configuration.

- At `serve` start, if `churches` holds **more than one** row, the server refuses to start with exit code 7 and: "This database contains N churches. The community edition serves exactly one. Use one install per church, or the hosted edition." CLI commands that need the church fail the same way.
- `/readyz` reports ready before setup (the server is ready to be set up).

The SaaS supplies its own resolver (slug from the path) through `server.WithTenantResolver`.

## 4. `URLBuilder`

Defined in `app`:

```go
type URLBuilder interface {
    // AppPath returns the browser path for an app route in the current church, e.g. "/members".
    AppPath(ctx context.Context, route string) string
    // AppURL returns the absolute URL for sharing (invite, reset, liturgy links).
    AppURL(ctx context.Context, route string) string
}
```

- `route` always starts with `/` and has no church prefix.
- Community implementation (`adapters/tenancy.NoPrefix`): `AppPath` returns `route` unchanged; `AppURL` returns `BaseURL + route`.
- The setup link is built with `AppURL` on a context without a tenant; the community implementation does not need one.
- Every link in API responses comes from `URLBuilder`. The web app builds links only through its own router helpers, never by concatenating strings with a church slug ([05 §5](05-web-shell.md#5-links-and-routing)).

## 5. Authorization

**Actor.** Each church-scoped use case starts with `actor, err := authz.Actor(ctx)`:

1. No session → `ErrUnauthenticated` → 401 `unauthenticated`.
2. Load the membership of the session's user in the tenant church. None → `ErrNotFound` → **404 `not_found`** **[P-26]**.
3. Load the member's roles and compute the **effective scopes** (union of the roles' scopes).
4. Return `app.Actor{UserID, ChurchID, MembershipID, RoleIDs []domain.RoleID, Scopes domain.ScopeSet}`.

**Scope check.** `actor.Require(scope)` (or `RequireAny`) with a constant from `domain/scope.go`; missing → **403 `forbidden`** **[P-26]**. *(Resolves "403 or 404" in [SPEC.md §8.1](../SPEC.md#81-tenancy) rule 4.)* Code never checks role names or `origin`. The scope list and safeguards are in [03 §8](03-identity-auth.md#8-member-roles-and-permissions).

**Visibility rule.** For each resource type, use cases define who can **see** it (e.g. from step 3: drafts are invisible to members without `liturgy.edit`, `liturgy.comment` or `liturgy.approve`).
- Cannot see the resource → **404 `not_found`**, as if it didn't exist.
- Can see it but lacks the scope for the action → **403 `forbidden`**.

**Indistinguishable 404s.** "Missing", "not a member" and "not visible" return byte-identical problem bodies. The reason is logged at info level as `not_found_reason` = `missing` \| `not_member` \| `not_visible`, with the user ID and request ID.

**Allowed actions.** Every resource in a response carries `actions`, computed by the same functions the use cases use:

```json
{ "id": "01J…", "name": "Budi", "roles": [{ "id": "01J…", "name": "Liturgist" }],
  "actions": { "edit_roles": true, "remove": true, "create_reset_link": true } }
```

**Actions are advisory only:** they help the UI show the right buttons, but they never authorize anything. Every mutation re-loads the actor's membership, roles and scopes and re-checks the safeguards inside its own write transaction before changing data. An action is `true` only if the scope check **and** the safeguards would pass (e.g. `remove` is `false` for the only member holding `roles.manage` and `members.manage`, or for a member whose roles hold scopes the viewer lacks). The UI never decides permissions itself.

## 6. Step-1 church and member API

| Method & path | Scope | Request | Response |
|---|---|---|---|
| `GET /api/v1/me` | session | — | `{ user: {id, name, email, phone, preferences}, membership: {id, roles, scopes, actions} \| null, church: {id, name, default_ui_language, …} \| null }` |
| `PATCH /api/v1/me` | session | `{ name?, preferences?: {text_size?: "normal"\|"large"\|"larger", ui_language?: "en"\|"id"\|null} }` | updated user |
| `POST /api/v1/me/password` | session | see [03 §9](03-identity-auth.md#9-password-reset) | 204 |
| `POST /api/v1/me/sessions/end-others` | session | — | 204 ([03 §4](03-identity-auth.md#4-sessions)) |
| `GET /api/v1/church` | baseline | — | church + `actions` |
| `PATCH /api/v1/church` | `church.settings` | `{ name?, default_ui_language?, default_language?, default_translation_code?, time_zone?, key_display?, feedback_url?, privacy_contact? }` | church |
| `GET /api/v1/members` | `members.view` | — | `{ members: [...], usage: { team_members: { used, max \| null } } }` |
| `PATCH /api/v1/members/{membershipId}` | `roles.manage` | `{ role_ids: [...] }` | member |
| `DELETE /api/v1/members/{membershipId}` | `members.manage` | — | 204 |
| `POST /api/v1/members/{membershipId}/password-reset` | `members.manage` | — | `{ link, expires_at }` |
| `GET /api/v1/roles` | `roles.manage` or `members.manage` | — | `[{ id, name, description, origin, scopes, member_count, actions }]` |
| `POST /api/v1/roles` | `roles.manage` | `{ name, description?, scopes: [...] }` | role |
| `PATCH /api/v1/roles/{id}` | `roles.manage` | `{ name?, description?, scopes? }` | role |
| `DELETE /api/v1/roles/{id}` | `roles.manage` | — | 204 |
| `GET /api/v1/scopes` | `roles.manage` or `members.manage` | — | `[{ scope, description }]` (description in the caller's UI language) |
| `GET /api/v1/invites` | `members.manage` | — | pending and expired invites (no links) |
| `POST /api/v1/invites` | `members.manage` | see [03 §7](03-identity-auth.md#7-invites) | invite + link |
| `POST /api/v1/invites/{id}/regenerate` | `members.manage` | — | `{ link, expires_at }` |
| `DELETE /api/v1/invites/{id}` | `members.manage` | — | 204 |
| `GET /api/v1/translations` | public | — | `[{ code, name, language }]` |

Every mutating role and member operation also applies the safeguards in [03 §8](03-identity-auth.md#8-member-roles-and-permissions).

- `usage.team_members.used` = memberships + pending invites; `max` is `null` when unlimited (community).
- **PATCH semantics** (all `PATCH` operations): an **omitted** field is left unchanged. `null` is allowed only where stated and means "clear / use the default": `preferences.ui_language: null` (follow the church default), `feedback_url: null`, `privacy_contact: null`. Empty strings are rejected (`validation_failed`) except `feedback_url: ""` and `privacy_contact: ""`, which also clear. Unknown fields → 422.
- **Changing one's own email or phone** is not possible in step 1 (deferred by the owner, review round 2). Mistakes are corrected when accepting the invite ([03 §7](03-identity-auth.md#7-invites)).
- `feedback_url` must be an absolute `https` URL or empty; `privacy_contact` is free text ≤ 500 characters ([SPEC.md §8](../SPEC.md#8-non-functional-requirements) privacy).

## 7. Extension points

All interfaces are defined in `app` in step 1 and are **provisional [P-27]**: signatures may change until the step that first uses them, without counting as a breaking change for `liturgist-saas` (module version `v0.x`).

| Interface | Step-1 signature | Community implementation in step 1 | First real use |
|---|---|---|---|
| `Entitlements` | see §8 | `adapters/entitlements/unlimited` | Step 1 (invites) |
| `AuthProvider` | `Authenticate(ctx, identifier, password string) (domain.UserID, error)` | none: password login stays in `app.Auth` ([03 §5](03-identity-auth.md#5-login-and-throttling)) until a second provider needs the port | SSO, passkeys (later) |
| `Storage` | `Put(ctx, key string, r io.Reader) error`, `Open(ctx, key string) (io.ReadCloser, error)`, `Delete(ctx, key string) error` | `adapters/storage/localfs` under `<DataDir>/files`; keys are `[a-z0-9/_.-]` only, no `..` | Logo upload (later step) |
| `EventBus` | `Publish(ctx, topic string, payload []byte) error`, `Subscribe(ctx, topic string) (<-chan []byte, func())` | `adapters/eventbus/memory` | Liturgy editor (step 3) |
| `Notifier` | `Compose(ctx, event NotifyEvent) ([]Message, error)` | `adapters/notify/copyshare` returning an empty list | Publishing (step 5) |
| `BibleTextProvider` | `Lookup(ctx, ref domain.Reference, translation string) (BibleText, error)` | none registered; returns `ErrNotAvailable` | Readings (step 2) |
| `Exporter` | `Export(ctx, liturgy PublishedVersion, format string, w io.Writer) error` | none | Step 5 |
| `Importer` | `Parse(ctx, r io.Reader, hint ImportHint) ([]ImportCandidate, error)` | none | Step 2 |

Types these ports use that later steps design (`domain.Reference`, `BibleText`, `NotifyEvent`, `Message`, `PublishedVersion`, `ImportHint`, `ImportCandidate`) are empty placeholder structs in step 1. Step 2 defines `domain.Reference` ([07 §2](07-readings.md#2-references)), `BibleText` ([07 §3.1](07-readings.md#31-lookup-order-and-storing-provider-text-p-50)), `ImportHint` and `ImportCandidate` ([08 §4](08-import.md#4-formats)); the importers for `paste`, `openlyrics` and `chordpro` are registered by `server`, and `server.WithImporter(format, Importer)` adds or replaces one. `server.WithBibleTextProvider(p)` appends a `BibleTextProvider` ([07 §3.1](07-readings.md#31-lookup-order-and-storing-provider-text-p-50)).

Community behaviour: `localfs` keys are at most 512 bytes and have no empty, `.` or `..` segments; a file is written to a temporary name and renamed when complete; `Open` of a missing key → `app.ErrNotFound`; `Delete` of a missing key succeeds. The `memory` EventBus gives each subscriber a 64-message buffer; when it is full, that subscriber misses messages instead of blocking the publisher, so listeners must be able to re-read the current state.

Rules ([SPEC.md §8.2](../SPEC.md#82-extensibility-and-editions)): use cases depend only on these interfaces; implementations are chosen in `server`; each `server.With…` option replaces the community default.

**Stability promise to `liturgist-saas`:** until v1.0 of this module, any port (every interface in `app` and `httpapi.TenantResolver`) may change; the SaaS pins exact `v0.x` tags. Every change to a port's signature or meaning is listed in `CHANGELOG.md` under a **Ports** heading for that release.

## 8. Entitlements

```go
type Feature string
const (
    FeatureAutomaticNotifications Feature = "automatic_notifications" // WhatsApp Business API, email sending
    FeatureLicensedBibleText      Feature = "licensed_bible_text"
    FeatureObjectStorage          Feature = "object_storage"
    FeatureSSOLogin               Feature = "sso_login"
    FeatureAIImport               Feature = "ai_import"
)

type LimitName string
const (
    LimitMaxActiveLiturgies      LimitName = "max_active_liturgies"
    LimitMaxUnpublishedLiturgies LimitName = "max_unpublished_liturgies"
    LimitMaxTeamMembers          LimitName = "max_team_members"
)

type Limit struct {
    Unlimited bool
    Max       int // meaningful only when !Unlimited
}

type Entitlements interface {
    Has(ctx context.Context, church domain.ChurchID, f Feature) (bool, error)
    Limit(ctx context.Context, church domain.ChurchID, l LimitName) (Limit, error)
}
```

- Community stub: `Has` → `true`; `Limit` → `Limit{Unlimited: true}`.
- Feature and limit constants live only in `app/entitlements.go`.
- Step-1 call site: invite creation and regeneration of an expired invite ([03 §7](03-identity-auth.md#7-invites)). The liturgy-limit call sites come with liturgies in step 3.
- An `Entitlements` error is treated as `unavailable` (never as "allowed").

## 9. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Read the church ID from the request in a handler or use case | `app.TenantFrom(ctx)` set by the middleware | One resolver; the SaaS swaps it |
| Return 403 when the caller isn't a member | 404 `not_found` **[P-26]** | Doesn't reveal what exists |
| Compute `actions` in the web app | Use the `actions` from the API | All rules stay in Go (decisions log) |
| Build links by string concatenation | `URLBuilder` | The SaaS adds `/<slug>` |
| Check `plan == "free"` or similar in feature code | `Entitlements.Has` / `.Limit` with constants | Edition vs entitlement separation ([SPEC.md §8.2](../SPEC.md#82-extensibility-and-editions)) |
| Treat an `Entitlements` error as allowed | Fail closed with `unavailable` | A broken plan service must not lift limits silently |
| Check role names or `origin` in code (`if role.Name == "Liturgist"`) | Check scopes only | Churches rename and redefine roles |

## 10. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-T-001 | Scope table | Each step-1 operation | Requires exactly the scope in §6 | Baseline operations need only membership |
| TC-T-002 | `actions` for a member | Viewer is the only member holding `roles.manage` + `members.manage` | Viewer's own row: `remove: false`; a team member's row: `remove: true` | Target holds a scope the viewer lacks → `remove: false`, `edit_roles: false`; viewer with only `members.view` → all `false` |
| TC-T-003 | `NoPrefix` URLBuilder | `AppURL("/invite")`, base `https://x.org` | `https://x.org/invite` | Base with trailing slash |
| TC-T-004 | `SingleChurch` resolver | No church | `ErrNotSetUp`; after `Refresh` with one church → its ID | Cached value used without DB access; two churches in the database → start-up fails with exit 7 |
| TC-T-005 | Unlimited entitlements | Any feature / limit | `true` / `Unlimited` | — |
| TC-T-006 | `localfs` keys | `../etc/passwd` | Rejected | Upper-case letters rejected |

### Integration tests (HTTP through `httptest`, temp-file SQLite)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-T-001 | Not a member | User U with no membership, logged in | Every church endpoint in §6 → 404 `not_found`, body identical to a request for a non-existent member ID; log line has `not_found_reason=not_member`; `GET /me` → `membership: null`, `church: null` | — |
| IT-T-002 | Missing scope | Logged-in team member (no roles) | `PATCH /church`, `GET /members`, `POST /invites`, `POST /roles` → 403 `forbidden` | Custom role with only `members.view` → `GET /members` 200, `POST /invites` 403 |
| IT-T-007 | Role editor safeguards | Church admin A and liturgist L | A removes `roles.manage` from Church admin role while A is the only holder → 409 `lockout_prevented`; L (no `roles.manage`) can't edit roles; after `liturgy.approve` is removed from the Church admin role, A creates a role with it → 403 `scope_not_held` | — |
| IT-T-003 | Not set up | Fresh install, logged out | `GET /api/v1/church` → 409 `not_set_up`; `GET /api/v1/setup/status` → 200 | — |
| IT-T-004 | Removed member | Member M removed by admin | M's next request → 404; M's session still valid for `GET /me` | — |
| IT-T-005 | Limit through entitlements | `WithEntitlements` stub with `max_team_members = 1`, 1 member | `GET /members` shows `used:1, max:1`; invite → 403 `limit_reached` with `limit`, `used`, `max` | — |
| IT-T-008 | Tenancy declarations | All registered operations | Each has the expected `tenancy`; an operation added through `WithRoutes` without a declaration is treated as `church` | — |
| IT-T-006 | SaaS-style resolver | Test resolver returning church B for path prefix `/b`, two churches | Member of A calling B's endpoints → 404 (community tests the foundation; slug routing is tested in the SaaS) | — |

## 11. Error handling matrix

| Error | Detection | Response | Fallback | Logging |
|---|---|---|---|---|
| No session | Actor step 1 | 401 `unauthenticated` | Web app redirects to login with return path | none |
| Not a member | Actor step 2 | 404 `not_found` | Web app shows "You are no longer a member of this church" when `GET /me` has no membership | info |
| Missing scope | `Require` | 403 `forbidden` | — | info |
| Not set up | Resolver | 409 `not_set_up` | Web app redirects to `/setup` | none |
| Resolver failure | Resolver error | 503 `unavailable` | Retry | error |
| Entitlements failure | Error from `Has`/`Limit` | 503 `unavailable` | Retry | error |
| Storage key invalid | Key validation | `app.ErrInvalid` → 422 | — | warn |

## 12. References

| Topic | Location |
|---|---|
| Tenancy rules | [SPEC.md §8.1](../SPEC.md#81-tenancy) |
| Extension points, editions, entitlements | [SPEC.md §8.2](../SPEC.md#82-extensibility-and-editions), [§8.2.1](../SPEC.md#821-usage-limits-saas-free-plan) |
| Allowed-actions decision | [SPEC.md §11](../SPEC.md#11-decisions-log) — row "The API returns the actions the current user may take" |
| Tenancy split (decision) | [SPEC.md §11](../SPEC.md#11-decisions-log) — row "Tenancy split" |
| Store and `ForChurch` | [02-persistence.md §2](02-persistence.md#2-ports-in-app) |
| Roles, scopes, safeguards | [SPEC.md §4](../SPEC.md#4-users-and-roles-mvp), [03-identity-auth.md §8](03-identity-auth.md#8-member-roles-and-permissions) |
| Error codes | [01-foundation.md §10](01-foundation.md#10-error-format-and-codes) |
