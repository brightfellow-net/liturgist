# Implementation Documents — Index (Reference)

> **Document type: Reference.** Index of the implementation documents and the decisions they propose.
> Strategy and product decisions live in [SPEC.md](../SPEC.md). These documents say **how** to build each step.

## 1. Documents for step 1

[SPEC.md §10](../SPEC.md#10-suggested-build-order), step 1: project skeleton, data model, migrations (SQLite + PostgreSQL), auth, church + users + roles, tenant context, `TenantResolver` and `URLBuilder` ports, membership tests, extension-point interfaces and the entitlement stub.

| Document | Covers |
|---|---|
| [01-foundation.md](01-foundation.md) | Repository layout, toolchain, build, configuration, CLI, `server` builder, HTTP basics, logging, errors, CI |
| [02-persistence.md](02-persistence.md) | Shared SQL adapter, dialects, transactions, type mapping, migrations, contract tests |
| [03-identity-auth.md](03-identity-auth.md) | Users, identifiers, passwords, sessions, CSRF, login throttling, invites, password resets, first-time setup, member roles and permissions |
| [04-tenancy-extensions.md](04-tenancy-extensions.md) | Tenant context, `TenantResolver`, `URLBuilder`, authorization, allowed actions, extension-point interfaces, entitlements |
| [05-web-shell.md](05-web-shell.md) | React app shell for step 1: pages, routing, API client, i18n |
| [../reference/schema.md](../reference/schema.md) | Database tables for step 1 (Reference) |

## 1a. Documents for step 2

[SPEC.md §10](../SPEC.md#10-suggested-build-order), step 2: song library (CRUD, sections, search) and readings store, plus importing songs. **Status: Approved** 2026-10-03 (drafted 2026-10-02; Spec Gate re-score and adversarial review round 3 done, [§6](#6-review-log)); coding of step 2 may start.

| Document | Covers |
|---|---|
| [06-song-library.md](06-song-library.md) | Songs, sections, default arrangements, language groups, copyright fields, search, library pages (slice 2A) |
| [07-readings.md](07-readings.md) | Bible reference parser, readings store, `BibleTextProvider` lookup, readings pages (slice 2B) |
| [08-import.md](08-import.md) | Paste and split, OpenLyrics, ChordPro, batches and candidate review, `Importer` (slice 2C) |
| [../reference/schema.md](../reference/schema.md#step-2-tables) | Step 2 tables (Reference) |

## 1b. Documents for step 3

[SPEC.md §10](../SPEC.md#10-suggested-build-order), step 3: templates and the weekly liturgy editor (items, reorder, songs, readings, assignments). **Status: decisions Approved** 2026-10-03 (Spec Gate and an adversarial review have run; conditions in §4b). Spec Gate score stays below 9 until Q-3.3 is closed. Built in four slices: 3A planning setup, 3B liturgies, 3C liturgy pages, 3D undo and redo (live updates and presence are on the icebox since 2026-10-03).

| Document | Covers |
|---|---|
| [09-planning.md](09-planning.md) | Duties, singing parts, templates, services, seeded defaults, their pages (slice 3A) |
| [10-liturgy.md](10-liturgy.md) | Liturgies, creating and preparing a week, items, songs and sequences, readings, assignments, versions, history, usage ports, limits (slice 3B) |
| [11-liturgy-editor.md](11-liturgy-editor.md) | Liturgy pages and the editor (slice 3C); undo and redo (slice 3D); the iceboxed live-updates design |
| [../reference/schema.md](../reference/schema.md#step-3-tables) | Step 3 tables (Reference) |

## 1c. Documents for step 4

[SPEC.md §10](../SPEC.md#10-suggested-build-order), step 4: review workflow up to Approved, item-level comments, state history. **Status: decisions Approved** 2026-10-04 (Spec Gate 9.0/10, self-scored; adversarial review round 4 done). Built in two slices: 4A states and history (**merged** 2026-10-04, [12 §7.1](12-review.md#71-slice-4a-as-built-2026-10-04)), 4B comments (**merged** 2026-10-04, [12 §7.2](12-review.md#72-slice-4b-as-built-2026-10-04)); step 4 is complete. Publishing is step 5 (owner decision 2026-10-04).

| Document | Covers |
|---|---|
| [12-review.md](12-review.md) | Transitions, notes, history, the undo floor, comments, pages |
| [../reference/schema.md](../reference/schema.md#step-4-tables) | Step 4 tables (Reference) |

## 1d. Documents for step 5

[SPEC.md §10](../SPEC.md#10-suggested-build-order), step 5: publishing with `PublishedVersion`, the published view, "my assignments", the print view, reading mode with offline use, and WhatsApp messages. **Status: decisions Approved** 2026-10-05 (Spec Gate 9.0/10, self-scored; adversarial review round 5 done). Built in five slices: 5A publishing (**merged** 2026-10-05, [13 §11.2](13-publishing.md#112-slice-5a-as-built-2026-10-05)), 5B reading views (**merged** 2026-10-05, [13 §11.3](13-publishing.md#113-slice-5b-as-built-2026-10-05)), 5C print (**merged** 2026-10-05, [13 §11.4](13-publishing.md#114-slice-5c-as-built-2026-10-05)), 5D reading mode and offline (**merged** 2026-10-05, [13 §11.5](13-publishing.md#115-slice-5d-as-built-2026-10-05)), 5E WhatsApp messages.

| Document | Covers |
|---|---|
| [13-publishing.md](13-publishing.md) | Publish, versions, reopen and archive, published routes, print, reading mode, messages, pages |
| [../reference/schema.md](../reference/schema.md#step-5-tables) | Step 5 tables (Reference) |

## 2. How to use these documents

- **Required reading before writing any step-1 code:** [SPEC.md](../SPEC.md) and **every** document in the table above. This index alone is not a specification.
- **Precedence [P-36]:**
  1. SPEC.md sections 1–10 decide **what** to build (product behaviour).
  2. Approved documents in `docs/impl/` and `docs/reference/` decide **how** (implementation details) and win over SPEC.md on those details.
  3. The SPEC.md decisions log (section 11) is history only.
  4. If two current documents still conflict, treat it as a bug: stop and ask the owner. Don't pick one.

## 3. Status

**All proposals approved on 2026-10-02 by the project owner (Hutomo Widjaja), in review sessions recorded through commit `d7cded9` (P-01 to P-32), the commit adding P-33 to P-36, and the commit adding P-37 to P-43.** Every decision these documents add on top of SPEC.md is marked **[P-xx]** and listed below; the markers stay as cross-references.

- Step-2 proposals P-44 to P-53 were approved by the project owner on 2026-10-03, after the Spec Gate and review round 3.
- New decisions found later are added here as **Proposed** until the owner approves them.
- If the **substance** of an approved proposal changes, its status returns to Proposed until re-approved. Wording fixes and clarifications that don't change behaviour don't need re-approval.

## 4. Proposed decisions

| ID | Proposal | Where | Status |
|---|---|---|---|
| P-01 | Go 1.27 toolchain (`go 1.27` in `go.mod`); golangci-lint **v2**, pinned in CI | [01 §2](01-foundation.md#2-toolchain) | **Approved** 2026-10-02 |
| P-02 | License header: two SPDX lines at the top of every source file | [01 §2](01-foundation.md#2-toolchain) | **Approved** 2026-10-02 |
| P-03 | Step-1 configuration: environment variables only (list in 01 §5); the optional config file comes with step 6 | [01 §5](01-foundation.md#5-configuration) | **Approved** 2026-10-02 |
| P-04 | Step-1 CLI commands: `serve`, `migrate`, `setup`, `user reset-password`, `openapi`, `version` | [01 §6](01-foundation.md#6-command-line) | **Approved** 2026-10-02 |
| P-05 | Secret tokens (invite, reset, setup) travel only in URL **fragments** (`#t=…`), never in paths or query strings; `Referrer-Policy: same-origin` | [01 §8](01-foundation.md#8-http-basics) | **Approved** 2026-10-02 |
| P-06 | Errors use RFC 9457 problem details plus a stable `code` field (list in 01 §10) | [01 §10](01-foundation.md#10-error-format-and-codes) | **Approved** 2026-10-02 |
| P-07 | Generated OpenAPI spec and TypeScript types are committed; CI fails if regenerating changes them | [01 §4](01-foundation.md#4-build-and-generated-files) | **Approved** 2026-10-02 |
| P-08 | `web/dist` is embedded with `//go:embed all:dist`; without a built frontend the server serves a "frontend not built" page | [01 §4](01-foundation.md#4-build-and-generated-files) | **Approved** 2026-10-02 |
| P-09 | SQLite uses two connection pools: 1 writer (immediate transactions) and 4 read-only readers; PostgreSQL uses one pool of 10 | [02 §3](02-persistence.md#3-connections-and-transactions) | **Approved** 2026-10-02 |
| P-10 | Column type mapping: ULIDs as text; timestamps as fixed-format UTC text (SQLite) / `timestamptz` (PostgreSQL); JSON as text / `jsonb`; enums as text with `CHECK` | [02 §4](02-persistence.md#4-type-mapping) | **Approved** 2026-10-02 |
| P-11 | Migrations are forward-only (no down migrations); automatic migration on start with downgrade protection; the SQLite pre-upgrade copy is built in step 1; expand/contract rule for removals and renames; `--allow-newer-schema` override (off by default) | [02 §5](02-persistence.md#5-migrations) | **Approved** 2026-10-02 |
| P-12 | Use-case tests use a SQLite file in a temporary folder, not `:memory:` (same pools, WAL and locking as production), copied from a migrated template file per test package. Amends the "in-memory" wording in the decisions log | [02 §6](02-persistence.md#6-testing-strategy) | **Approved** 2026-10-02 |
| P-13 | Roles are defined per church from fixed scopes; members hold any number of roles; no role = team member; three editable ready-made roles (Church admin, Liturgist, Editor); role editor in step 1 | [SPEC §4](../SPEC.md#4-users-and-roles-mvp), [03 §8](03-identity-auth.md#8-member-roles-and-permissions) | **Approved** 2026-10-02 |
| P-14 | Rename `RoleType` to **duties** (`Duty`); duties grant no permissions | [SPEC §7](../SPEC.md#7-data-model-sketch) | **Approved** 2026-10-02 |
| P-15 | Scope list (10 scopes incl. `liturgy.manage`) and the scopes of the ready-made roles | [SPEC §4](../SPEC.md#4-users-and-roles-mvp) | **Approved** 2026-10-02 (including `liturgy.manage`); amended 2026-10-02: Church admin holds all scopes ([SPEC §11](../SPEC.md#11-decisions-log)) |
| P-16 | Safeguards: no lock-out (someone always holds `roles.manage` + `members.manage`), no escalation, scope checks only | [SPEC §4](../SPEC.md#4-users-and-roles-mvp), [03 §8](03-identity-auth.md#8-member-roles-and-permissions) | **Approved** 2026-10-02 |
| P-17 | Passwords: argon2id m=19456 KiB, t=2, p=1; at most 2 hashes at once; re-hash on login when settings change; 10–128 characters after NFKC normalisation, not trimmed; rejected if in the embedded 100,000 common-password list, a curated Indonesian/church word list, or equal to the person's identifier, own name or the church's name; "show password" button on every password field | [03 §3](03-identity-auth.md#3-passwords) | **Approved** 2026-10-02 |
| P-18 | Login throttling stored in the database with three counters: identifier+IP 5 failures/15 min → 15 min lock; identifier 50 failures/1 h → 1 h lock; IP 100 failures/15 min → 15 min lock. Trusted proxies (`LITURGIST_TRUSTED_PROXIES`, optional `LITURGIST_CLIENT_IP_HEADER`) in step 1. IPs kept only while a counter is active; mentioned in the privacy notice | [03 §5](03-identity-auth.md#5-login-and-throttling), [01 §5](01-foundation.md#5-configuration) | **Approved** 2026-10-02 |
| P-19 | Sessions: 32-byte random token, stored only as SHA-256; cookie `__Host-liturgist_session` on HTTPS, `liturgist_session` on plain HTTP; 90 days extended on use (written at most hourly) with an absolute maximum of 1 year; every login/accept/setup/reset creates a new token and deletes a session the browser already had; changing the password ends other sessions, a reset ends all; "Log out on all other devices" in step 1 | [03 §4](03-identity-auth.md#4-sessions) | **Approved** 2026-10-02 |
| P-20 | CSRF protection without tokens: Go's `http.CrossOriginProtection` (Sec-Fetch-Site, then Origin; requests with neither header allowed as non-browser clients) with the base URL added as a trusted origin, plus our own rule that unsafe requests with a body must be JSON; SameSite=Lax cookie | [03 §6](03-identity-auth.md#6-csrf-protection) | **Approved** 2026-10-02 |
| P-21 | Invites: name + at least one identifier; 7 days; one pending invite per identifier per church; "resend" regenerates the link; link shown only at creation/resend; new invitees confirm and may correct name, email and phone; if the invited identifier belongs to an account only that account can accept, otherwise the invitee creates an account or accepts with the account they are logged in with; invites stay valid if their creator is removed | [03 §7](03-identity-auth.md#7-invites) | **Approved** 2026-10-02 |
| P-22 | Reset links: 24 hours, single use, a new link cancels older unused ones; admin-created only for members whose roles' scopes the admin holds and who belong to no other church; the reset page names the creator; admins see the latest reset link per member (creator, time, used); creation and use logged by user ID; "Forgot password?" on the login page explains to ask the church admin; email reset later | [03 §9](03-identity-auth.md#9-password-reset) | **Approved** 2026-10-02 |
| P-23 | `liturgist user reset-password` prints a reset link (never takes a password), warns when `LITURGIST_BASE_URL` is unset; plus `liturgist user list` and the emergency `liturgist member grant-admin <identifier>` (gives the ready-made Church admin role, recreating it if deleted; logged) | [01 §6](01-foundation.md#6-command-line), [03 §9](03-identity-auth.md#9-password-reset) | **Approved** 2026-10-02 |
| P-24 | Setup token stored as a hash in the database, valid 24 hours; each start while not set up, and `liturgist setup-link`, replace it and print the link in a framed block; no localhost shortcut. Step-1 wizard: church name, first admin, default UI language (English pre-selected), content language, translation, time zone (default WIB), key display; services added in step 3; `/setup` after setup says "already set up". Later steps' migrations seed their defaults into existing churches | [03 §10](03-identity-auth.md#10-first-time-setup), [schema](../reference/schema.md#setup_tokens) | **Approved** 2026-10-02 |
| P-25 | The community `TenantResolver` returns the install's only church from the database (cached, refreshed after setup), not from config; the server refuses to start (exit 7) if the database holds more than one church. Amends SPEC §8.1 rule 2 | [04 §3](04-tenancy-extensions.md#3-tenantresolver) | **Approved** 2026-10-02 |
| P-26 | 401 when not logged in; 404 `not_found` when not a member or when the member can't see the resource; 403 `forbidden` when the member can see it but lacks the scope for the action. All 404s have identical bodies; the reason (`not_member`, `not_visible`, `missing`) is only logged. Resolves SPEC §8.1 rule 4 | [04 §5](04-tenancy-extensions.md#5-authorization) | **Approved** 2026-10-02 |
| P-27 | All extension-point interfaces are defined in step 1 and marked provisional: signatures may change until the step that first uses them. Stability promise: before v1.0 any port may change; every port change is listed under "Ports" in `CHANGELOG.md` | [04 §7](04-tenancy-extensions.md#7-extension-points) | **Approved** 2026-10-02 |
| P-28 | Step-1 web pages: setup, login, accept invite, reset password, home, profile, church settings, members & invites, privacy notice | [05 §2](05-web-shell.md#2-pages-in-step-1) | **Approved** 2026-10-02 |
| P-29 | PostgreSQL tests run in testcontainers (`postgres:17-alpine`) when `LITURGIST_TEST_POSTGRES=1`; CI always sets it | [02 §6](02-persistence.md#6-testing-strategy) | **Approved** 2026-10-02 |
| P-30 | Security headers on every response (CSP, `nosniff`, `frame-ancestors 'none'`, `Referrer-Policy`) | [01 §8](01-foundation.md#8-http-basics) | **Approved** 2026-10-02 |
| P-31 | Translations are a global table seeded by migration (TB, TB2, BIS, CUV, KJV, WEB), created in step 1 because the setup wizard asks for a default translation | [reference/schema.md](../reference/schema.md#translations) | **Approved** 2026-10-02 |
| P-32 | Expired sessions, invites, reset links and throttle rows are deleted by an hourly cleanup job | [03 §11](03-identity-auth.md#11-cleanup-job) | **Approved** 2026-10-02 |
| P-33 | CSRF: every `POST`, `PUT`, `PATCH`, `DELETE` under `/api/` must have `Content-Type: application/json`, with or without a body (adversarial review #6) | [03 §6](03-identity-auth.md#6-csrf-protection) | **Approved** 2026-10-02 |
| P-34 | Role and member changes take the per-church lock (`LockChurch`) before checking the safeguards, so concurrent changes can't lock everyone out on PostgreSQL (review #10) | [03 §8](03-identity-auth.md#8-member-roles-and-permissions), [02 §3](02-persistence.md#3-connections-and-transactions) | **Approved** 2026-10-02 |
| P-35 | `liturgist auth clear-throttle` command, and a once-per-start warning when proxy headers arrive but `LITURGIST_TRUSTED_PROXIES` is not set (review #12) | [03 §5](03-identity-auth.md#5-login-and-throttling) | **Approved** 2026-10-02 |
| P-36 | Precedence between SPEC.md and these documents (§2) (review #2) | [§2](#2-how-to-use-these-documents) | **Approved** 2026-10-02 |
| P-37 | Atomic operations for every "check, then write" rule (atomic claim, atomic counters, conditional session update, `LockChurch`/`LockUser`/`LockInstall`), backed by partial unique indexes and checks; one retry list with backoff; 10 s transaction deadlines; microsecond time precision; race tests on both databases (review round 2) | [02 §2.1](02-persistence.md#21-atomic-operations), [schema](../reference/schema.md) | **Approved** 2026-10-02 |
| P-38 | The binary listens on `127.0.0.1:8080` by default (the Docker image sets `:8080`); start-up warnings for plain HTTP on the network and for `https` without TLS | [01 §5](01-foundation.md#5-configuration) | **Approved** 2026-10-02 |
| P-39 | Allowed-host check (`BaseURL` host plus `LITURGIST_EXTRA_HOSTS`), three supported deployment setups, no forwarded scheme/host headers trusted, no CORS | [01 §5.1](01-foundation.md#51-supported-deployment-setups), [01 §8](01-foundation.md#8-http-basics) | **Approved** 2026-10-02 |
| P-40 | Invite roles in an `invite_roles` table; invites refer to live roles (edits before acceptance apply, as for existing holders) | [03 §7](03-identity-auth.md#7-invites), [schema](../reference/schema.md#invite_roles) | **Approved** 2026-10-02 |
| P-41 | Optional strict upgrade mode `LITURGIST_REQUIRE_PREUPGRADE_COPY=true`; default stays "skip the copy with a warning" | [02 §5](02-persistence.md#5-migrations) | **Approved** 2026-10-02 |
| P-42 | The setup link stays in the log and stderr: accepted risk, with reasons recorded | [03 §10](03-identity-auth.md#10-first-time-setup) | **Approved** 2026-10-02 (risk accepted by the owner) |
| P-43 | Changing one's own email or phone is deferred beyond step 1 | [04 §6](04-tenancy-extensions.md#6-step-1-church-and-member-api) | **Approved** 2026-10-02 (deferred) |
| P-44 | Step 2 is built in three slices: 2A songs, 2B readings, 2C import (paste and split, OpenLyrics, ChordPro). Usage history and report, singing parts and church licence settings wait for steps 3 and 5 | [06 §1](06-song-library.md#1-scope), [08 §1](08-import.md#1-scope) | **Approved** 2026-10-03 (scope chosen by the owner 2026-10-02) |
| P-45 | Every member views songs and readings; `library.edit` creates, changes, deletes and imports | [06 §3](06-song-library.md#3-api), [07 §4](07-readings.md#4-api) | **Approved** 2026-10-03 (chosen by the owner 2026-10-02) |
| P-46 | Sections are replaced as one ordered list with stable IDs and request-local keys for new ones; the default arrangement is a table whose foreign keys guarantee that every entry names a section of the same song; songs and readings carry a `version` (+1 per request that changes the row, its sections, its arrangement or its group membership; 409 `version_conflict`); a song group has at least two songs and one song per language | [06 §2](06-song-library.md#2-data-model) | **Approved** 2026-10-03 |
| P-47 | Songs and readings are deleted for good after a confirmation; `SongUsage` and `ReadingUsage` ports report liturgy use (always "unused" in step 2) | [06 §6](06-song-library.md#6-ports), [07 §6](07-readings.md#6-ports) | **Approved** 2026-10-03 (chosen by the owner 2026-10-02) |
| P-48 | Search: one folding function with a fixed Unicode order and a token contract (letters, digits, single spaces), prefix search on a normalised index for Indonesian and English, substring search for Chinese, canonical hymnal keys, three result tiers without relevance scores, bytewise ordering, index updated in the same transaction, `liturgist search reindex` atomic per church; identical ordered results on both databases are a contract test | [06 §5](06-song-library.md#5-search) | **Approved** 2026-10-03 (no stemming accepted by the owner 2026-10-02) |
| P-49 | Reference parser: USFM book codes, Indonesian names and LAI abbreviations in canonical order with ordinals, 100-character input limit, structure checks only, one-chapter books, unsupported forms rejected with a reason; Indonesian book names shown in both UI languages for now | [07 §2](07-readings.md#2-references) | **Approved** 2026-10-03 (book table and Indonesian-only names accepted by the owner 2026-10-02; aliases may be extended from the pilot church's documents) |
| P-50 | Readings are unique per church + reference + translation; lookup order stored reading → providers (5 s each, failures skipped) → nothing; provider text is stored only by the server (`POST /readings/from-provider`, `may_store` checked there) and manual readings never carry a provider label; attribution suggested from the last reading in the translation | [07 §3](07-readings.md#3-the-readings-store) | **Approved** 2026-10-03 |
| P-51 | Import goes through batches and candidates with explicit decision/outcome states (a failed candidate is retryable; a batch closes when every candidate is terminal); text is sent as JSON with stated per-file, aggregate and complexity limits; Apply runs one transaction per candidate under the church lock through the song use cases; merge shows a preview, matches sections deterministically, keeps unmatched sections unless the member ticks removal, and refuses a target that changed after the decision; batches expire after 7 days | [08 §2–§5](08-import.md#2-flow-p-51) | **Approved** 2026-10-03 (the merge rule, as changed after the adversarial review, re-confirmed by the owner) |
| P-52 | Sections and arrangements are reordered with Move up / Move down buttons, not drag-and-drop alone | [06 §4](06-song-library.md#4-pages) | **Approved** 2026-10-03 |
| P-53 | Hymnal numbers are not unique; the form warns about duplicates | [06 §2.1](06-song-library.md#21-song) | **Approved** 2026-10-03 |

## 4b. Proposed decisions (step 3)

**Approved** by the owner on 2026-10-03, after the adversarial review of the step 3 documents and the owner's answers in §4c. P-56, P-61, P-65 and P-66 carry the conditions written in their rows.

| ID | Proposal | Where | Status |
|---|---|---|---|
| P-54 | Step 3 is built in four slices: 3A planning setup (duties, singing parts, templates, services), 3B liturgies (backend), 3C liturgy pages, 3D per-person undo and redo (amended 2026-10-03: live updates and presence moved to the icebox, owner). Review states, comments, publishing and the archive button of "Prepare next week" wait for steps 4–5 | [09 §1](09-planning.md#1-scope), [10 §1](10-liturgy.md#1-scope), [11 §1](11-liturgy-editor.md#1-scope) | **Approved** 2026-10-03 |
| P-55 | Duties, singing parts, templates and services are edited with `templates.edit`; every member can read duties and singing parts; templates and services are readable by `templates.edit` or `liturgy.edit` (403 otherwise). Names are unique per church by folded key; a duty or part in use cannot be deleted | [09 §2](09-planning.md#2-data-model), [§4](09-planning.md#4-api) | **Approved** 2026-10-03 |
| P-56 | Defaults (duties, singing parts, a starter template) are seeded in the church's content language at setup and, for existing churches, once at start-up by `app.Seed` guarded by a marker table; a church that deletes a default never gets it back. Replaces "the migration seeds the defaults" of P-24 for these rows | [09 §3](09-planning.md#3-seeded-defaults-p-56) | **Approved** 2026-10-03 — mechanism only; the seeded words stay Proposed until the Q-3.3 gate is closed |
| P-57 | Templates and services are replaced as whole lists (items, times) with a `version`; template items have no stable IDs and nothing refers to them | [09 §2](09-planning.md#2-data-model) | **Approved** 2026-10-03 |
| P-58 | A liturgy is created from a service and/or template by copying; its language is fixed at creation and its template must have the same language; it keeps a copy of the service name; at most one liturgy per service, date and time (two services may share a slot: owner decision Q-3.7) | [10 §2.1](10-liturgy.md#21-liturgy), [§3](10-liturgy.md#3-creating-a-liturgy) | **Approved** 2026-10-03 |
| P-59 | "Prepare next week": the week is Monday to Sunday in the church's time zone; occurrences come from the services' weekly times; creation is all-or-nothing and checks the limits once for the batch | [10 §3.1](10-liturgy.md#31-prepare-next-week-p-59) | **Approved** 2026-10-03 |
| P-60 | Two counters: the liturgy `version` guards structure (which items, their order, the liturgy's fields), each item's `version` guards that item's content; assignments are unversioned; conflicts are 409 `version_conflict` with a `scope` | [10 §5](10-liturgy.md#5-versions-and-conflicts-p-60) | **Approved** 2026-10-03 |
| P-61 | An item's type cannot be changed after creation; references to songs, readings and sections are cleared (with title snapshots) when a song, reading or section that only published liturgies use is deleted; `problems` are computed, not enforced, in step 3 | [10 §2.2](10-liturgy.md#22-item), [§6](10-liturgy.md#6-usage-ports-and-deleted-songs-readings-sections) | **Approved** 2026-10-03 — the rule for reopening a liturgy with cleared references is decided in step 4 |
| P-62 | Adding a song fills its sequence on the server from the default arrangement, else all sections in order (not only verses); the songs of an item are replaced as one list | [10 §2.3](10-liturgy.md#23-item-song-and-sequence) | **Approved** 2026-10-03 |
| P-63 | Assignments are a member or a free-text name per duty, several per duty, unique per duty and person, kept (flagged, computed from current membership) after a member leaves; `GET /liturgies/assignable` lists every member's id and display name only, for `liturgy.edit` (owner decision Q-3.8) | [10 §2.4](10-liturgy.md#24-assignment-p-63) | **Approved** 2026-10-03 |
| P-64 | Unpublished liturgies are visible only to members holding `liturgy.edit`, `liturgy.comment`, `liturgy.approve` or `liturgy.manage`; others get 404 | [10 §2.1](10-liturgy.md#21-liturgy) | **Approved** 2026-10-03 |
| P-65 | Every change writes a history row with before and after images in the same transaction (slice 3B); undo and redo are per person, refused when **another person's** history row touched the item or structure since (not by comparing version numbers), are history rows themselves and never targets, with a 50-edit window (slice 3D); history images have no size cap. **Amended 2026-10-03 by the second review:** a refused non-structural undo is skipped afterwards, a refused redo drops the person's redo stack, a re-created item gets a version above any it had, a state change of the review workflow sets an undo floor, and assignment undo reports a member who left as `reference_gone` | [10 §7](10-liturgy.md#7-history), [11 §7.2](11-liturgy-editor.md#72-undo-and-redo-p-65) | **Approved** 2026-10-03; re-approved 2026-10-03 with these amendments (owner: "agree with all"). Condition met: the review was a model of the rules over 20,000 random two-user histories (0 violations; weakened rules fail), see 11 §7.2; the dialect-level behaviour is covered by TC-U-007 and IT-E-003 in slice 3D |
| P-66 | Live updates are server-sent events carrying IDs and versions only; presence is tracked per connection from heartbeats on the event bus and expires after 60 seconds; the stream re-checks authorization every 60 seconds (a stated bound); a sixth stream of one member is refused with 429, not served by closing another | [11 §7.1](11-liturgy-editor.md#71-server-sent-events-p-66) | **Iceboxed** 2026-10-03 (owner: "put the live update feature on icebox"); was Approved the same day. The proxy spike ran first: a stream with a 20-second keepalive held 130 s behind nginx and Caddy; see [11 §7.1](11-liturgy-editor.md#71-live-updates-and-presence-icebox-p-66). Nothing is built; the row stays as the design to start from |
| P-67 | Keys are stored as letters; the web app shows "Do = G" or "G" by the church setting, and "La = Em" for minor keys in "Do" mode | [11 §2](11-liturgy-editor.md#2-key-display-p-67) | **Approved** 2026-10-03 |

## 4c. Questions for the owner (step 3)

These need an answer, not just an approval; the draft shows the assumed default.

| ID | Question | Draft assumes |
|---|---|---|
| Q-3.1 | Are live updates, presence and undo (slice 3D) part of step 3, or do they wait until after the pilot starts? SPEC §5.2 lists them in the MVP | In step 3, as the last slice; the earlier slices work without it. **Decided 2026-10-03 (owner):** yes, as drafted. If slice 3D grows, presence is cut before undo. **Amended 2026-10-03 (owner):** live updates and presence are on the icebox; slice 3D is undo and redo only |
| Q-3.2 | Should the setup wizard also ask for regular services? SPEC §5.7 says yes; the wizard built in step 1 does not | No: the Services page has a helpful empty state. **Decided 2026-10-03 (owner):** no wizard step; this deviates from SPEC §5.7 and is logged in the SPEC decisions log (2026-10-03) |
| Q-3.3 | What is GKY Citragarden's real order of service, and who checks the English and Chinese words of the seeded duties, parts and starter template? | The draft's generic order and words ([09 §3](09-planning.md#3-seeded-defaults-p-56)) **Decided 2026-10-03 (owner):** coding is not blocked; the generic seed stays. **Open gate before the pilot:** the real order of service of GKY Citragarden and a native reader for each of the English and two Chinese word sets. Until both are given, the Spec Gate stays below 9. |
| Q-3.4 | Which scope edits duties and singing parts: `templates.edit` or `church.settings`? | `templates.edit`, so a liturgist-type role can add a duty without being a settings admin. **Decided 2026-10-03 (owner):** `templates.edit`, as drafted |
| Q-3.5 | May an assigned team member see an unpublished liturgy they are assigned to? | No, only members with a `liturgy.*` scope (P-64). **Decided 2026-10-03 (owner):** no, as drafted. Team members see liturgies only once published; "my assignments" is built on published liturgies in step 5. Showing a member their own assignment in a draft (read-only) may be added later and changes no stored data |
| Q-3.6 | Is "La = Em" the right way to show a minor key in "Do" mode for your churches? | Yes. **Decided 2026-10-03 (owner):** yes, as drafted, to be checked by one musician at the pilot church; a different convention is a display change only (keys are stored as letters) |
| Q-3.7 | May two different services share the same date and time (for example two rooms)? A manual time that is not one of the service's times counts as a separate slot | Yes: the slot key is (service, date, time) and only the same service is limited ([10 §2.1](10-liturgy.md#21-liturgy)). **Decided 2026-10-03 (owner):** yes, as drafted. A manual time that is not one of the service's times is a separate slot; a typo can therefore make a near-duplicate, which is accepted |
| Q-3.8 | May every holder of `liturgy.edit` see the names of all church members (to assign them), even without `members.view`? Only the display name is returned | Yes ([10 §4](10-liturgy.md#4-editing-items-songs-and-assignments)). **Decided 2026-10-03 (owner):** yes, as drafted: id and display name of every member, nothing else. No opt-out for now |
| Q-3.9 | Should renaming a duty change the wording of drafts at once (published versions stay as published)? | Yes, live names; the published copy freezes them ([09 §2.1](09-planning.md#21-duties-and-singing-parts-p-55)). **Decided 2026-10-03 (owner):** yes, as drafted. **Requirement for step 5:** a published version stores the displayed duty names, not only the IDs |

## 4d. Proposed decisions (step 4)

**Approved** by the owner on 2026-10-04, after the Spec Gate (9.0/10, self-scored) and review round 4. The owner decided the scope (publishing in step 5, flat comments, self-approval allowed, open comments only warn), answered Q-4.1 to Q-4.3, and accepted every HIGH finding of round 4 as fixed in the draft, including the hard 500-comment cap.

| ID | Proposal | Where | Status |
|---|---|---|---|
| P-68 | Four transition routes (`submit`, `approve`, `request-changes`, `reopen`) with the scopes `liturgy.edit` / `liturgy.approve`, an optional note (0–500 characters), and one conditional update per transition on `state` and `edit_seq`; Approved → Published is step 5 | [12 §2](12-review.md#2-states-and-transitions-p-68), [§3](12-review.md#3-api-p-68) | **Approved** 2026-10-04 |
| P-69 | Submit is refused for a liturgy with no items (`empty_liturgy`) or with any `problem` of [10 §2.5](10-liturgy.md#25-problems) (`has_problems`); approving never re-checks | [12 §2](12-review.md#2-states-and-transitions-p-68) | **Approved** 2026-10-04 |
| P-70 | Approve and request changes state the `edit_seq` the reviewer saw; a mismatch is 409 `review_stale` | [12 §2](12-review.md#2-states-and-transitions-p-68) | **Approved** 2026-10-04 |
| P-71 | The history sequence update (`NextSeq`) becomes conditional on an editable state and returns `ErrNoSeq` on zero rows, which one helper turns into 404 or 409 `liturgy_locked`, so an edit or undo cannot commit after a transition (changes the 3B and 3D code) | [12 §2](12-review.md#2-states-and-transitions-p-68) | **Approved** 2026-10-04 |
| P-72 | Flat comments (1–2,000 characters, at most 500 per liturgy), on an item or the whole liturgy; anyone with `liturgy.comment` may resolve or reopen any; no edit or delete; allowed in `draft`, `in_review`, `needs_revision`; kept when their item is removed (no foreign key, title snapshot), re-attached by undo | [12 §4](12-review.md#4-comments-p-72) | **Approved** 2026-10-04 |
| P-73 | Tables `liturgy_state_changes` (migration `00008_state_changes.sql`, slice 4A) and `liturgy_comments` (`00009_comments.sql`, slice 4B), both dialects | [12 §5](12-review.md#5-data-model-p-73), [schema](../reference/schema.md#step-4-tables) | **Approved** 2026-10-04 |
| P-74 | Comments and state changes are readable only with a `liturgy.*` scope, whatever the state; creating or resolving a comment first takes the liturgy row lock with a no-op update that re-checks the state, then counts, reads the item title and inserts | [12 §2](12-review.md#2-states-and-transitions-p-68), [§4](12-review.md#4-comments-p-72) | **Approved** 2026-10-04 |

### 4e. Questions for the owner (step 4)

| # | Question | Proposed answer |
|---|---|---|
| Q-4.1 | Should a submit be refused while an item is unfinished (a song item with no song, a reading item with no reading)? | Yes, so the reviewer never reads a half-filled order; the alternative is a warning only (P-69). **Decided 2026-10-04 (owner):** yes, refuse |
| Q-4.2 | Should the author be able to delete their own comment? | No: resolve it; deletion is not planned (P-72). **Decided 2026-10-04 (owner):** no |
| Q-4.3 | Should the reviewer's note on "request changes" be required? | No, optional (P-68). **Decided 2026-10-04 (owner):** optional |

## 4f. Decisions (step 5)

**Approved** by the owner on 2026-10-05, after the Spec Gate (9.0/10, self-scored) and review round 5, as written. The owner decided the scope (everything in SPEC §5.6 for the pilot, print by stylesheet only, a generic print layout until the GKY document is available, reopen allowed with cleared references shown as problems). Q-5.1 to Q-5.4 were answered the same day.

| ID | Proposal | Where | Status |
|---|---|---|---|
| P-75 | `publish` is the fifth transition (`approved` → `published`, `liturgy.approve`, `edit_seq` required); the copy is made in the same transaction, so a published liturgy always has a version; `Transition` also requires `archived_at IS NULL`; problems are re-checked and the stored content is capped at 4 MiB; a liturgy with any version can never be deleted (conditional `DELETE`, foreign key `NO ACTION`) | [13 §2](13-publishing.md#2-publishing-p-75) | **Approved** 2026-10-05 |
| P-76 | `PublishedVersion` is an immutable, self-contained JSON copy (format 1) with displayed duty and person names, credit lines and the licence footer text as at publishing; sections stored once per item song | [13 §3](13-publishing.md#3-publishedversion-p-76) | **Approved** 2026-10-05 |
| P-77 | A published liturgy can be reopened to `draft` (limit `max_unpublished_liturgies`, under `LockChurch`); members keep seeing the last version marked "being revised"; cleared references (P-61) do not block it and show as problems | [13 §4](13-publishing.md#4-reopening-archiving-unarchiving-p-77-p-78) | **Approved** 2026-10-05 (the owner chose "allow") |
| P-78 | Archive and unarchive (`liturgy.manage`): archive only from `published`, not a state change; unarchive checks `max_active_liturgies`; the archive-and-create button of "Prepare next week" is built | [13 §4](13-publishing.md#4-reopening-archiving-unarchiving-p-77-p-78) | **Approved** 2026-10-05 |
| P-79 | Every member of the church can read the latest published copy of any liturgy that has one (state and archive flag aside), through routes of their own; **the `published` exception of `openLiturgy` and the list is removed**, so members without a `liturgy.*` scope never reach the editable liturgy or its history | [13 §5](13-publishing.md#5-reading-the-published-copy-p-79-p-80) | **Approved** 2026-10-05 |
| P-80 | "My assignments" are computed from the latest versions of upcoming, non-archived liturgies (nearest 50, matched through `published_assignees`), not from the editable assignments | [13 §5](13-publishing.md#5-reading-the-published-copy-p-79-p-80) | **Approved** 2026-10-05 |
| P-81 | The print view renders from the stored copy with print CSS (A4, F4; team and musician variants; options with church defaults in `churches.settings`); no server-side PDF in step 5 | [13 §6](13-publishing.md#6-print-view-p-81) | **Approved** 2026-10-05 (print by stylesheet chosen by the owner) |
| P-82 | Reading mode (highlighted own items, "Go to my part", Wake Lock), a service worker caching the app shell and the last five opened published liturgies, cleared at logout and on 401, and a web app manifest | [13 §7](13-publishing.md#7-reading-mode-and-offline-use-p-82) | **Approved** 2026-10-05 |
| P-83 | The WhatsApp texts are built in the web app from structured data in the liturgy's language; recipients carry phone numbers for `liturgy.approve` only; the change summary compares items by ID, songs by song, assignments by duty and person, and never reports text edits | [13 §8](13-publishing.md#8-whatsapp-messages-and-change-summary-p-83) | **Approved** 2026-10-05 |
| P-84 | Migration `00010_published.sql` (`published_versions`, `published_assignees`); no copy of the latest version on `liturgies`; five slices 5A to 5E | [13 §9](13-publishing.md#9-data-model-p-84), [§11](13-publishing.md#11-slices) | **Approved** 2026-10-05 |

Required changes to earlier documents were listed in [13 §11.1](13-publishing.md#111-changes-to-earlier-documents-applied-in-the-docs-commit-that-follows-the-owners-approval); they were applied on approval.

### 4g. Questions for the owner (step 5)

| # | Question | Proposed answer |
|---|---|---|
| Q-5.1 | Should the church logo in the print header (upload through `Storage`) be part of step 5? | No: the header shows the church name; the logo needs an upload route, type and size checks and a serving route, and waits until after the first pilot week ([13 §1](13-publishing.md#1-scope)). **Decided 2026-10-05 (owner):** yes, as proposed |
| Q-5.2 | May a member with `liturgy.approve` see the phone numbers of the people assigned, so the app can open a WhatsApp chat with each? | Yes, only in the recipients of the message page, only for people still members; the alternative is a Copy button only ([13 §8](13-publishing.md#8-whatsapp-messages-and-change-summary-p-83)). **Decided 2026-10-05 (owner):** yes, as proposed |
| Q-5.3 | Should "my assignments" list liturgies from today onward only? | Yes: today and later in the church's time zone; past services are in the published list ([13 §5](13-publishing.md#5-reading-the-published-copy-p-79-p-80)). **Decided 2026-10-05 (owner):** yes, as proposed |
| Q-5.4 | May the offline cache hold the five most recently opened published liturgies on a member's phone? | Yes, cleared at logout and mentioned in the privacy notice; the alternative is no offline use ([13 §7](13-publishing.md#7-reading-mode-and-offline-use-p-82)). **Decided 2026-10-05 (owner):** yes, as proposed |
| Q-5.5 | Is a generic print layout acceptable for the pilot start, to be restyled when GKY Citragarden's document arrives (open decision 3)? | Yes (the owner said so on 2026-10-05). **Decided 2026-10-05 (owner)** |

## 5. Process

1. Owner approves, changes or rejects each P-xx.
2. Spec Gate re-score (target 9/10 or above).
3. Adversarial review by a different AI model or a human reviewer, using the prompt in the stream-coding skill. Fix every CRITICAL finding; record a fix/accept/defer decision for each HIGH finding. **The owner decides** every accept/defer; each needs a written reason and, for a deferral, the step in which it will be handled.
   - **Exit condition:** coding may start when no CRITICAL finding is open and every HIGH finding has a recorded decision.
4. Code for step 1, following these documents. Any change found necessary while coding is made in these documents first.

## 6. Review log

| Round | Date | Scope reviewed | Result |
|---|---|---|---|
| 1 | 2026-10-02 | This index only ([adversarial-review.md](adversarial-review.md)); the linked documents were not reviewed | 2 CRITICAL, 7 HIGH, 7 MEDIUM. Fixed: #1, #2, #3, #4, #6, #8, #10, #12, #13, #14, #16 (P-33 to P-36 plus clarifications). Clarified, already covered: #5, #7, #11. No change, already covered elsewhere: #9 (SPEC §8.1, 04 §3), #15 (SPEC §8, §5.2). No open CRITICAL or HIGH |
| 2 | 2026-10-02 | 01–05 ([implementation-adversarial-review.md](implementation-adversarial-review.md)) and the schema reference ([schema-adversarial-review.md](../reference/schema-adversarial-review.md)) | Implementation: 3 CRITICAL, 13 HIGH, 14 MEDIUM. Schema: 2 CRITICAL, 8 HIGH, 8 MEDIUM. All CRITICAL fixed (atomic claims for invites, resets and setup; setup token singleton; throttle key encodings). HIGH: all fixed (P-37 to P-40 and clarifications) except the setup-link-in-logs finding, **accepted** as P-42 with reasons. MEDIUM: all fixed or clarified; changing one's own email/phone **deferred** (P-43). No open CRITICAL; every HIGH has a decision |
| 3 | 2026-10-03 | Step 2 documents 06, 07, 08 and the step 2 schema ([step2-adversarial-review.md](step2-adversarial-review.md)); reviewed by another model | 2 CRITICAL, 12 HIGH, 10 MEDIUM. **CRITICAL fixed:** failed imports vs. batch closing (candidate states, retry, closing rule: 08 §2.1); client-bypassable provider storage rule (server-side `from-provider`, 07 §3.1). **HIGH, all fixed in the documents (none deferred):** search parity contract and ordered-ID tests, canonical hymnal keys, merge preview and non-destructive default, stale merge target, versions on link/unlink, arrangement table with foreign keys, per-file/aggregate/complexity import limits, deterministic section matching, failure granularity for multi-song files, search tiers and per-dialect queries, paste repeat mapping with examples, ChordPro directive list. **MEDIUM fixed:** fold order, arrangement representation, book ordinals, parser input limit, atomic reindex, candidate edits vs. Apply, state machine, provider timeouts, authorization per route, migration split. P-51 merge rule re-confirmed by the owner 2026-10-03. No open CRITICAL or HIGH |
| 4 | 2026-10-04 | Step 4 document 12 and the step 4 schema; reviewed by a fresh Claude session with the adversarial prompt (same provider, which the stream-coding rule discourages) | 2 CRITICAL, 7 HIGH, 8 MEDIUM, 4 LOW. **CRITICAL fixed in the draft:** the conditional `NextSeq` would have answered 404/500 for a locked liturgy (`ErrNoSeq` and the `nextSeq` helper, P-71); submit could bypass the problems check in a race (update conditional on `state` and `edit_seq`, 12 §2). **HIGH fixed in the draft, accepted by the owner 2026-10-04:** review data visible on published liturgies (P-74), comment locking and cap, the approve warning source, order of checks, migration split (`00008`/`00009`), comment id scoping. **MEDIUM and LOW** fixed in the draft. The claims about `NextSeq`, `record`, `undo.step` and `openLiturgy` were checked against the code; nothing was run |
| 5 | 2026-10-05 | Step 5 document 13 and the step 5 schema; reviewed by a fresh Claude session with the adversarial prompt (same provider again) | 2 CRITICAL, 8 HIGH, 9 MEDIUM, 4 LOW. **CRITICAL fixed in the draft (both checked against the code):** `Delete` read the state and then deleted unconditionally, so a delete racing a publish, or a delete of a reopened liturgy, could destroy the copy (conditional `DELETE`, `NO ACTION` foreign key, no delete once a version exists); `openLiturgy` and the list let members without a scope read the editable view and the edit history of a `published` liturgy (the exception is removed, 13 §5). **HIGH fixed in the draft:** the size cap and how it is measured, the offline design (cache marker header, two caches, clearing rules), no `ETag`, the locked limit check on reopen and its messages, `published_assignees` for "my assignments", the church settings code and PATCH rules, the archive-and-create contract. **MEDIUM and LOW** fixed (archive predicates and logs, `Compare` "moved" by longest common subsequence, label language, `summary` headers and privacy text, route parameters, navigation, message length blocks, tests). **Owner:** accepted the fix plan 2026-10-05 ("ok go"), including refusing delete once any version exists (a deviation from SPEC §8.2.1 rule 3, to be logged in the SPEC decisions log on approval). Nothing was run |
