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

[SPEC.md §10](../SPEC.md#10-suggested-build-order), step 2: song library (CRUD, sections, search) and readings store, plus importing songs. **Status: Proposed** (drafted 2026-10-02); coding starts after the owner approves P-44 to P-53, the Spec Gate re-score and the adversarial review ([§5](#5-process)).

| Document | Covers |
|---|---|
| [06-song-library.md](06-song-library.md) | Songs, sections, default arrangements, language groups, copyright fields, search, library pages (slice 2A) |
| [07-readings.md](07-readings.md) | Bible reference parser, readings store, `BibleTextProvider` lookup, readings pages (slice 2B) |
| [08-import.md](08-import.md) | Paste and split, OpenLyrics, ChordPro, batches and candidate review, `Importer` (slice 2C) |
| [../reference/schema.md](../reference/schema.md#step-2-tables) | Step 2 tables (Reference) |

## 2. How to use these documents

- **Required reading before writing any step-1 code:** [SPEC.md](../SPEC.md) and **every** document in the table above. This index alone is not a specification.
- **Precedence [P-36]:**
  1. SPEC.md sections 1–10 decide **what** to build (product behaviour).
  2. Approved documents in `docs/impl/` and `docs/reference/` decide **how** (implementation details) and win over SPEC.md on those details.
  3. The SPEC.md decisions log (section 11) is history only.
  4. If two current documents still conflict, treat it as a bug: stop and ask the owner. Don't pick one.

## 3. Status

**All proposals approved on 2026-10-02 by the project owner (Hutomo Widjaja), in review sessions recorded through commit `d7cded9` (P-01 to P-32), the commit adding P-33 to P-36, and the commit adding P-37 to P-43.** Every decision these documents add on top of SPEC.md is marked **[P-xx]** and listed below; the markers stay as cross-references.

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
| P-44 | Step 2 is built in three slices: 2A songs, 2B readings, 2C import (paste and split, OpenLyrics, ChordPro). Usage history and report, singing parts and church licence settings wait for steps 3 and 5 | [06 §1](06-song-library.md#1-scope), [08 §1](08-import.md#1-scope) | **Proposed** (scope chosen by the owner 2026-10-02) |
| P-45 | Every member views songs and readings; `library.edit` creates, changes, deletes and imports | [06 §3](06-song-library.md#3-api), [07 §4](07-readings.md#4-api) | **Proposed** (chosen by the owner 2026-10-02) |
| P-46 | Sections are replaced as one ordered list with stable IDs; songs and readings carry a `version` checked on every change (409 `version_conflict`); removing a section removes it from the arrangement; a song group has at least two songs and one song per language | [06 §2](06-song-library.md#2-data-model) | **Proposed** |
| P-47 | Songs and readings are deleted for good after a confirmation; `SongUsage` and `ReadingUsage` ports report liturgy use (always "unused" in step 2) | [06 §6](06-song-library.md#6-ports), [07 §6](07-readings.md#6-ports) | **Proposed** (chosen by the owner 2026-10-02) |
| P-48 | Search: one folding function (no stemming), full-text prefix search on a normalised index for Indonesian and English, substring search for Chinese, hymnal-number lookup first, index updated in the same transaction, `liturgist search reindex` as repair | [06 §5](06-song-library.md#5-search) | **Proposed** (no stemming accepted by the owner 2026-10-02) |
| P-49 | Reference parser: USFM book codes, Indonesian names and LAI abbreviations, structure checks only, one-chapter books, unsupported forms rejected with a reason; Indonesian book names shown in both UI languages for now | [07 §2](07-readings.md#2-references) | **Proposed** (book table and Indonesian-only names accepted by the owner 2026-10-02; aliases may be extended from the pilot church's documents) |
| P-50 | Readings are unique per church + reference + translation; lookup order stored reading → providers → nothing; provider text with `may_store = false` is never saved; attribution suggested from the last reading in the translation | [07 §3](07-readings.md#3-the-readings-store) | **Proposed** |
| P-51 | Import goes through batches and candidates; text is sent as JSON (6 MiB limit for `POST /imports` only); Apply runs one transaction per candidate through the song use cases; merge fills empty fields and replaces sections keeping matching IDs; batches expire after 7 days | [08 §2–§5](08-import.md#2-flow-p-51) | **Proposed** (merge rule accepted by the owner 2026-10-02) |
| P-52 | Sections and arrangements are reordered with Move up / Move down buttons, not drag-and-drop alone | [06 §4](06-song-library.md#4-pages) | **Proposed** |
| P-53 | Hymnal numbers are not unique; the form warns about duplicates | [06 §2.1](06-song-library.md#21-song) | **Proposed** |

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
