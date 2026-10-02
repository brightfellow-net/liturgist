# Adversarial Review — Database Schema Reference

Reviewed document: [`schema.md`](schema.md)  
Review date: 2026-10-02

This review concerns the schema reference and its stated cross-database constraints. Findings distinguish database-enforced invariants from rules left to application code.

## Critical

### [CRITICAL] The schema cannot represent all three specified throttle counters

**Location:** `auth_throttle` table; `03-identity-auth.md` §5 and proposal P-18

**Problem:** The auth design requires counters keyed by identifier+IP, identifier, and IP. The schema describes `key` as only `id:<identifier>` or `ip:<address>`. It gives no representation for identifier+IP, and a developer must invent a key encoding or collapse counters. Either choice can change rate-limit behavior and allow bypasses or unintended shared lockouts.

**Fix:** Specify canonical, collision-safe key encodings for all three counter kinds (including normalization and IPv6 handling), or use separate typed key columns with a composite/unique key. Align the identity document, proposal summary, and schema.

### [CRITICAL] “At most one” setup token is not a database invariant

**Location:** `setup_tokens` table header and key definition; `03-identity-auth.md` §10

**Problem:** The header claims “At most one row exists,” but `token_hash` as the primary key permits any number of rows. Concurrent startup or setup-link creation can insert multiple rows. The application document also describes delete-then-insert without a serialization mechanism, so the schema does not prevent multiple live setup credentials or nondeterministic invalidation.

**Fix:** Encode the singleton explicitly, such as a fixed singleton key with a `CHECK`, and require atomic replace/claim semantics in the transaction. Test parallel issuance and consumption on both databases.

## High

### [HIGH] Community single-church invariant is not represented in the schema

**Location:** `churches` table; `04-tenancy-extensions.md` §3

**Problem:** The community resolver and setup flow require exactly one church at most, but `churches` has an ordinary ULID primary key and no singleton constraint. Concurrent PostgreSQL setup transactions can both pass a count check and insert separate rows, leaving a database the community server refuses to start against.

**Fix:** Add an enforceable singleton mechanism for community databases, or make setup acquire a database-wide serialization lock and prove that mechanism is used by every setup entry point, including CLI and HTTP.

### [HIGH] Required default translation can be stored as NULL

**Location:** `churches.default_translation_id`

**Problem:** Setup requires a `default_translation_code` and the church settings model treats a default translation as part of the configuration, but the column is nullable. A bug, direct SQL operation, or partial migration can leave a church without a default translation and force every consumer to invent fallback behavior.

**Fix:** Make the column `NOT NULL` if a default is mandatory. If null is a valid state, specify its meaning and the fallback behavior in the data model and API.

### [HIGH] Reset-token single-use and “one at a time” are not schema-enforced

**Location:** `password_resets` table; `03-identity-auth.md` §9

**Problem:** The schema uniquely identifies each token but permits multiple unused, unexpired reset rows for one user. It also does not prevent two transactions from consuming the same token concurrently. The application text promises one active reset and single-use semantics, but a table-level constraint or claim state is absent.

**Fix:** Add a partial unique index for one active reset per user where supported by both target databases (or a portable equivalent), and define an atomic conditional consume operation. Confirm SQLite and PostgreSQL migration syntax and behavior match.

### [HIGH] Invite status columns allow impossible state combinations

**Location:** `invites.accepted_at`, `accepted_user_id`, and `cancelled_at`

**Problem:** The table permits an invite to be both accepted and cancelled, to have an `accepted_user_id` without `accepted_at`, or to be marked accepted without an accepted user. Application code may prevent these states today, but the schema does not protect integrity from races, migrations, or future writers.

**Fix:** Add checks requiring accepted and cancelled states to be mutually exclusive and requiring `accepted_user_id` exactly when accepted, unless a documented deletion policy requires a nullable accepted user after acceptance. Define that deletion behavior explicitly.

### [HIGH] Invite role JSON is neither structurally validated nor immutable

**Location:** `invites.role_ids`

**Problem:** `role_ids` is a JSON array of IDs, but the schema only says JSON and has no element/type validation or foreign-key relationship. The implementation says deleted role IDs are skipped, while roles can be edited between invitation and acceptance. The value therefore does not actually preserve the authorization decision “at the time of inviting”; it points to mutable role records and may contain malformed or cross-church IDs.

**Fix:** Store invite role assignments in a normalized join table with a church-scoped composite FK and, if the intended grant is fixed at invite creation, snapshot the approved scopes or role version. Define behavior when roles are edited or deleted after creation.

### [HIGH] Pending-invite uniqueness relies on application serialization only

**Location:** `invites` table and open-invite index; `03-identity-auth.md` §7

**Problem:** The implementation promises one pending invite per identifier per church, but the schema has only a non-unique index on `(church_id, expires_at)`. It cannot prevent duplicate active invites with the same email or phone if a writer bypasses the intended church lock, or if lock enforcement regresses. A single uniqueness index cannot directly cover both nullable identifier columns with one rule.

**Fix:** Add portable partial unique indexes for non-null email and phone, scoped by church and active status, with clearly defined normalization and expiration semantics; otherwise explicitly make a transaction-level `LockChurch` the sole invariant and test every invite-writing path against it.

### [HIGH] Role scope strings are unconstrained at the database boundary

**Location:** `role_scopes.scope`

**Problem:** The reference explicitly omits a `CHECK` and relies on application validation. Any migration, SQL maintenance, or alternative writer can insert an unknown scope that may be ignored by current code or interpreted differently later. This can create silent permission loss or unintended permission behavior.

**Fix:** Choose and document a deliberate forward-compatibility policy. Either enforce known scopes with checks updated by migrations, or make unknown-scope handling explicit and fail closed when loading roles; add tests for unknown stored scopes.

### [HIGH] Database text constraints omit the declared token-hash shape

**Location:** `sessions.token_hash`, `invites.token_hash`, `password_resets.token_hash`, and `02-persistence.md` §4

**Problem:** Type mapping calls SHA-256 hashes 64 hex characters, but the schema lists only text/primary-key/unique constraints. Malformed hashes can be stored by migrations or direct writes and may behave inconsistently across lookup code.

**Fix:** Add matching length/hex checks in SQLite and PostgreSQL, or remove the claimed physical constraint from the type mapping and state that validation is application-only.

## Medium

### [MEDIUM] Timestamp ordering and state validity are largely unconstrained

**Location:** Sessions, invites, password_resets, and auth_throttle tables

**Problem:** The schema permits negative lifetimes and impossible time ordering, such as `expires_at < created_at`, `last_seen_at < created_at`, or throttle locks ending before their window begins. Several operational rules depend on these values and treat them as authoritative.

**Fix:** Add checks for invariant timestamp relationships where stable across both dialects, and identify which checks remain application-only. Include boundary and malformed-row tests.

### [MEDIUM] User identity format and role-name normalization are not protected in storage

**Location:** `users.email`, `users.phone`, `roles.name_key`

**Problem:** The schema says email is lower-case, phone is E.164, and `name_key` is the lower-cased trimmed role name, but these are application comments rather than constraints. Case variants or unnormalized values inserted by another writer can evade uniqueness or produce duplicate accounts/roles.

**Fix:** Define a canonicalization invariant and enforce it with portable checks or generated canonical columns where feasible. Otherwise require every write path to use the same domain constructors and add repository tests that attempt noncanonical inserts.

### [MEDIUM] Church-scoped identity rows are not uniformly constrained to a church on actor references

**Location:** `invites.created_by` and other church-owned rows referencing platform `users`

**Problem:** `created_by` is a platform FK only. This is intentional to preserve invites after creator removal, but it also permits a church-owned invite to name a user who never belonged to that church. The document does not distinguish valid historical attribution from invalid cross-tenant attribution.

**Fix:** State whether `created_by` is an audit label only or must refer to a member at creation. If it must, enforce/check membership at creation and retain the user ID after membership deletion without requiring a live composite FK.

### [MEDIUM] `role_scopes` and `membership_roles` lack explicit church-leading indexes for all access paths

**Location:** `role_scopes` and `membership_roles`

**Problem:** The conventions require every church-owned table to have an index starting with `church_id`, but `role_scopes` has only primary key `(role_id, scope)` and no explicit church-leading index. `membership_roles` does have one. The stated convention and actual table definition conflict, and role-scope listing/filtering can require scans.

**Fix:** Add an index beginning with `(church_id, role_id)` to `role_scopes`, or narrow the convention and document the query plan/index rationale.

### [MEDIUM] SQLite/PostgreSQL check equivalence is asserted but expressions are not specified

**Location:** Conventions; table constraints throughout

**Problem:** The document says constraint names are identical and refers to logical checks, but many constraints are prose (“1–120 chars”, “lower-case”, “E.164”) rather than portable SQL expressions. SQLite counts text length differently from Go Unicode code points in some edge cases, and PostgreSQL `length`/collation behavior may differ. There is no exact cross-dialect definition for equality or case folding.

**Fix:** For each invariant, state whether it is app-enforced or DB-enforced. For DB constraints, give equivalent SQLite and PostgreSQL expressions and test Unicode, case, and boundary values on both.

### [MEDIUM] `churches.default_translation_id` does not guarantee language compatibility

**Location:** `churches.default_language`, `churches.default_translation_id`; `translations`

**Problem:** The FK proves the translation exists but does not ensure its `language` matches `default_language`. A direct update or migration can pair an Indonesian default language with KJV or a Chinese language with TB, leaving consumers to resolve contradictory settings.

**Fix:** Define whether these fields are independent. If they must match, enforce the relation in application transactions and/or use a composite FK with a unique `(id, language)` key.

### [MEDIUM] JSON shape requirements differ between SQLite and PostgreSQL without a defined validator

**Location:** `users.preferences`, `churches.settings`, `invites.role_ids`; `02-persistence.md` §4

**Problem:** SQLite only requires `json_valid`, PostgreSQL uses `jsonb`, and the schema describes object or array shapes and preserved unknown keys without specifying validation timing or behavior for wrong top-level types. PostgreSQL `jsonb` also normalizes representation while SQLite text preserves it.

**Fix:** Define canonical JSON schemas and whether the adapter rejects invalid shape on both databases. Specify unknown-key preservation and semantic equality independent of serialized byte representation.

### [MEDIUM] The declared universal church-owned-table index rule has no query-driven exception process

**Location:** Conventions and `translations`/church-owned tables

**Problem:** The convention mandates an index starting with `church_id`, but the document does not clarify whether primary keys containing another leading column count, whether every table needs one even when tiny, or who may approve exceptions. Implementers can satisfy the prose differently.

**Fix:** Define what qualifies as a church-leading index and document any deliberate exceptions, including `role_scopes`.
