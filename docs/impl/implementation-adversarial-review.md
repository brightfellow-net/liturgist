# Adversarial Review — Step 1 Implementation Documents

Reviewed documents: `01-foundation.md` through `05-web-shell.md`  
Review date: 2026-10-02

This review targets the implementation requirements directly. Findings describe risks present in the documents; they do not assert that an implementation already exists or is vulnerable.

## Critical

### [CRITICAL] One invite can be accepted concurrently more than once

**Location:** `03-identity-auth.md` §7, “Accept as new user” and “Accept with the logged-in account”; `02-persistence.md` §2–3

**Problem:** Acceptance is described as checking that an invite is pending, creating a membership/user, and marking it accepted in a `Tx.Write`. No invite row lock, conditional state transition, or unique constraint is required. Under PostgreSQL `READ COMMITTED`, two transactions can both observe a pending invite and both create memberships (possibly for different accounts) before either acceptance becomes visible. “Token … single use” is therefore not guaranteed by the specified transaction boundary.

**Fix:** Require an atomic claim/consume operation: lock the invite row (`FOR UPDATE` on PostgreSQL), re-read state after acquiring the lock, and transition it from pending only once; make user/membership creation part of that same transaction. Specify an equivalent conditional update/serialization mechanism and test simultaneous acceptance on both dialects.

### [CRITICAL] Setup can create multiple churches under concurrent PostgreSQL requests

**Location:** `03-identity-auth.md` §10; `02-persistence.md` §2–3

**Problem:** Setup checks that no church exists and then inserts a church in a read-committed `Tx.Write`. There is no global setup lock, unique singleton constraint, or atomic conditional insert. Two requests using the setup token concurrently can both pass the count check and create two churches, contrary to the one-church community invariant. Deleting the token later in each transaction does not prevent both transactions from having read it as valid.

**Fix:** Enforce singleton setup in the database and atomically claim the setup token before creating records. Require the token claim and church creation to commit in one transaction, and specify/test the concurrent outcome on SQLite and PostgreSQL.

### [CRITICAL] Password reset token use is not specified as atomic

**Location:** `03-identity-auth.md` §9; `02-persistence.md` §2–3

**Problem:** Reset use checks unused/unexpired, updates the password, marks the token used, deletes sessions, and creates a session. The docs do not require locking or conditional consumption. Two concurrent uses can both pass the check, race to set different passwords, and both issue sessions despite the promised single-use token.

**Fix:** Atomically claim the unused token before changing credentials; only the transaction that changes its state may proceed. Keep password update, token consumption, session revocation, and new session creation in that transaction. Add a concurrent-use contract test for each database.

## High

### [HIGH] New password-reset links can race and leave multiple valid links

**Location:** `03-identity-auth.md` §9; `02-persistence.md` §2–3

**Problem:** Creating a link marks all unused links used and inserts a new one, but the user row or reset set is not locked and no uniqueness constraint for an active reset is specified. Concurrent requests can each invalidate the rows they saw and insert a valid token, violating “one at a time.”

**Fix:** Serialize reset creation per user or use a database-enforced active-reset invariant, with invalidation and insertion in one transaction. Define which request wins and test parallel creation on both dialects.

### [HIGH] Default deployment exposes the application over plain HTTP

**Location:** `01-foundation.md` §5 and §8; `03-identity-auth.md` §4

**Problem:** Defaults bind to `:8080`, set `BaseURL` to `http://localhost:8080`, and use a non-Secure session cookie. A self-host operator who exposes the default listener beyond loopback can send credentials and session cookies over cleartext HTTP. The docs do not require TLS termination, restrict the default listener, or refuse production-style non-loopback HTTP.

**Fix:** Define the supported deployment modes. At minimum, bind to loopback by default or require explicit acknowledgement for a non-loopback HTTP listener; document and validate the trusted TLS-terminating proxy setup, and ensure the public base URL and Secure-cookie behavior match it.

### [HIGH] Host-based CSRF allowance has no host allowlist

**Location:** `03-identity-auth.md` §6

**Problem:** When `Sec-Fetch-Site` is absent, requests are allowed if `Origin` matches the request host or a trusted origin. The docs do not constrain acceptable `Host` values. A hostile or misconfigured host/proxy path can make the request host itself attacker-controlled, weakening the same-origin comparison. Registering `BaseURL` as trusted does not define how untrusted Host headers are rejected.

**Fix:** Specify host validation at the proxy and application boundary. Compare Origin against a canonical configured origin and explicitly configured aliases; do not treat an arbitrary request Host as trusted. Define forwarded-host handling and tests for forged/mismatched Host and Origin values.

### [HIGH] Setup token rotation races across server instances

**Location:** `03-identity-auth.md` §10; `02-persistence.md` §3

**Problem:** Every server start while unset creates a token and deletes the previous one. If two instances start against the same PostgreSQL database, their startup operations can interleave, so each can print a link that is invalidated by the other. The documented “latest start” rule is not deterministic under concurrent starts.

**Fix:** Make setup-token issuance a serialized database operation with a defined owner/lease or make startup reuse the current unexpired token. State how operators retrieve the currently valid link in a multi-instance deployment.

### [HIGH] Role IDs in pending invites can gain scopes after approval

**Location:** `03-identity-auth.md` §7–8

**Problem:** Invite creation checks that the creator holds every scope in the selected roles, but acceptance later assigns the referenced role IDs and skips deleted roles. If a role is edited to add scopes after the invite is created, acceptance can grant the invitee scopes the creator never approved or no longer holds. No role snapshot, version check, or acceptance-time authorization is specified.

**Fix:** Define invite role semantics: freeze the role/scope set at creation, invalidate invites when referenced roles change, or revalidate against an authorized actor at acceptance. Do not silently apply mutable role definitions without an explicit rule.

### [HIGH] Identifier and IP throttles lack atomic counter-update semantics

**Location:** `03-identity-auth.md` §5; `02-persistence.md` §2–3

**Problem:** Throttling uses three counters and fixed thresholds, but the docs do not specify atomic increment/upsert behavior, how simultaneous failures are counted, or how overlapping windows are reset. Concurrent login attempts can lose increments or bypass a threshold if implemented as read-then-write operations.

**Fix:** Specify an atomic database operation for counter updates and lock decisions, with an explicit window key/anchor and reset policy. Test concurrent attempts around every threshold on both dialects.

### [HIGH] Invalid-identifier login path appears to bypass throttling

**Location:** `03-identity-auth.md` §5, login steps 1–2

**Problem:** Invalid identifiers are directed to run the dummy-hash step and then return `invalid_credentials` before the listed throttling check. If implemented literally, malformed identifiers are not counted or throttled, leaving an unbounded expensive Argon2 path that can exhaust the hash semaphore and degrade legitimate logins.

**Fix:** Apply IP throttling before identifier parsing, count malformed attempts against the IP counter, and run the dummy hash under the same bounded/cancellable work policy. Specify consistent responses without skipping abuse controls.

### [HIGH] PostgreSQL deadlock retry policy contradicts itself

**Location:** `02-persistence.md` §2 and §8

**Problem:** §2 says `Tx.Write` retries serialization failures (`40001`) or `SQLITE_BUSY`; §8 also maps PostgreSQL deadlocks (`40P01`) to a retry. Implementers can reasonably omit one behavior, and deadlocks may surface as unhandled errors or retries with differing side effects.

**Fix:** Give one authoritative retry list, maximum-attempt/backoff policy, and cancellation behavior. Define whether `40P01` retries the whole transaction and how retry exhaustion maps to the API.

### [HIGH] Request-scoped middleware does not define fail-closed behavior for platform routes

**Location:** `04-tenancy-extensions.md` §2; `01-foundation.md` §8

**Problem:** Tenant middleware has a hard-coded exception list of platform paths, including several invite operations. New routes added through `WithRoutes` receive middleware globally, but the documents do not define how an extension registers or is prohibited from becoming a platform operation. A path mismatch or newly added route could skip tenancy checks or accidentally require a tenant for an account-level flow.

**Fix:** Replace path-prefix exceptions with explicit route metadata or separate router groups. Specify that unknown routes default to tenant-scoped behavior and require explicit reviewed registration for platform operations.

### [HIGH] “Every scoped query filters by churchID” is a convention, not a demonstrated enforcement boundary

**Location:** `02-persistence.md` §2 and §7; `04-tenancy-extensions.md` §2 and §5

**Problem:** `ForChurch` promises scoped repositories filter by church ID, but SQLite has no RLS and PostgreSQL policies are explicitly absent in step 1. The contract tests exercise one invite list, not every repository method. A missed predicate in any repository can expose or mutate another church’s rows.

**Fix:** Require an automated contract test for every church-scoped repository method using two churches, and ensure composite foreign keys prevent cross-church writes. For PostgreSQL, define exactly when RLS policies are installed and how a missing tenant setting fails closed.

### [HIGH] URL-derived tenant and invite tenant mismatch behavior is underspecified in community mode

**Location:** `04-tenancy-extensions.md` §2–4

**Problem:** Invite operations “take the church from the invite itself” and compare against a resolved tenant only “if a tenant could also be resolved.” The community URL has no church prefix, while the SaaS may use a slug path. The docs do not define how the resolver extracts the request tenant for platform invite paths, leaving a developer to decide whether to trust the invite token alone or bind it to the path tenant.

**Fix:** Specify route-by-route tenant binding, including how the invite token is scoped to the SaaS slug and what happens when slug resolution fails or disagrees.

### [HIGH] Authentication cookies depend on external proxy correctness without a required topology

**Location:** `01-foundation.md` §5 and §8; `03-identity-auth.md` §4 and §6

**Problem:** `BaseURL` determines Secure cookies and trusted Origin, while the listener may be plain HTTP behind a proxy. The docs do not specify which headers the proxy must strip/set, whether the app trusts forwarded scheme/host headers, or how to prevent direct access bypassing the proxy. A wrong deployment can weaken cookie and origin protections without startup failure.

**Fix:** Define a supported proxy contract and validation checks. Reject unsafe/inconsistent external URL configuration and document network controls that prevent bypassing the TLS proxy.

### [HIGH] Session rehash/update races can resurrect or inconsistently extend sessions

**Location:** `03-identity-auth.md` §4–5 and §9; `02-persistence.md` §2–3

**Problem:** Authenticated requests extend sessions hourly, while password changes, resets, logout, and “end others” delete sessions. The docs do not specify conditional updates against the same session row or ordering semantics. Concurrent requests can race with revocation and may update a row after deletion/replacement or issue a cookie for a no-longer-valid session.

**Fix:** Define session extension as a conditional update of an unexpired row, and define revocation as winning over concurrent extension. Set cookies only after successful commit and test revocation/extension races.

## Medium

### [MEDIUM] SQLite migration backup retention can race across processes

**Location:** `02-persistence.md` §5

**Problem:** The pre-upgrade copy and “keep newest 3” cleanup have no SQLite migration lock or process-level exclusion described. Two processes can race to choose the same timestamped filename, copy an inconsistent pre-migration state, or delete each other’s retained backups.

**Fix:** Specify an exclusive migration lock spanning version check, backup, migration, and retention; define collision-proof backup names and cleanup ordering.

### [MEDIUM] SQLite free-space check is not a guarantee of safe backup

**Location:** `02-persistence.md` §5 and §11

**Problem:** Checking that free space is at least 1.2× the database file size is a snapshot estimate. Concurrent writes, WAL size, filesystem metadata, and database growth can consume space after the check. Continuing migrations after a skipped backup is specified, but no operator-visible confirmation or abort mode is required.

**Fix:** Include database and WAL/journal space in the estimate, handle copy failures explicitly, and provide a configurable strict mode that aborts migration when a pre-upgrade backup cannot be made.

### [MEDIUM] `Tx.Write` retry makes generated IDs and timestamps nondeterministic

**Location:** `02-persistence.md` §2 and §4

**Problem:** A retry reruns the whole use-case callback, but IDs and timestamps are supplied by app-level generators and there is no rule whether they are generated before or inside the callback. Different attempts may use different values, complicating audit consistency and any response prepared before commit.

**Fix:** Define retry-safe use-case structure: generate stable command IDs/timestamps before the retried transaction or guarantee that failed-attempt values are never externally visible and all outputs come from the committed attempt.

### [MEDIUM] Timestamp truncation can change expiration boundary decisions

**Location:** `02-persistence.md` §4 and test TC-P-001; `03-identity-auth.md` §11

**Problem:** SQLite timestamps are stored at microsecond precision and the test explicitly truncates nanoseconds, while expiration checks compare against `Clock.Now()`. The docs do not require normalization before computing and comparing expiry values. A token/session very near expiry can behave differently after a SQLite round trip than on PostgreSQL.

**Fix:** Define canonical timestamp precision for both dialects and normalize all persisted/comparison values to it before calculating lifetimes or deciding expiry.

### [MEDIUM] Client-IP parsing has no malformed or multi-value failure rule

**Location:** `01-foundation.md` §5; `03-identity-auth.md` §5 and TC-A-011

**Problem:** The docs give trusted-proxy examples but do not define behavior for malformed `X-Forwarded-For`, multiple header lines, whitespace, obfuscated values, or an invalid configured header value. Different parsers may choose different IPs, compromising throttle consistency and auditability.

**Fix:** Define a strict parsing algorithm from the trusted proxy chain, reject or ignore malformed values deterministically, and specify the fallback key when the client IP cannot be determined.

### [MEDIUM] Privacy text promises IP deletion “within about an hour” without matching cleanup schedule

**Location:** `05-web-shell.md` §2; `03-identity-auth.md` §5 and §11

**Problem:** The privacy page says IP addresses are deleted within about an hour, but throttle rows are cleaned hourly only after the window and any lock have ended. A lock can last an hour after the window, so stored IP-derived counter keys may persist longer than the public statement implies. The duration and representation of IP keys are also not stated.

**Fix:** Define retention from the actual throttle lifecycle and make the notice match it, or expire/anonymize IP-keyed rows within the promised period without breaking active lockouts.

### [MEDIUM] Cookie clearing and Secure-cookie migration behavior are not specified

**Location:** `03-identity-auth.md` §4

**Problem:** The cookie name changes between HTTP and HTTPS. The docs say to clear an expired/missing session cookie but do not say whether both cookie names are cleared when the deployment changes scheme. A stale insecure cookie can remain, and duplicate names can create ambiguous browser/server behavior during migration.

**Fix:** Define cookie deletion attributes and explicitly clear both names on authentication transitions and scheme changes; ensure the server rejects ambiguous duplicate session cookies.

### [MEDIUM] Explicit CORS policy and preflight response behavior are absent

**Location:** `03-identity-auth.md` §6; `01-foundation.md` §8

**Problem:** The CSRF argument relies on the API allowing no CORS, but the docs do not define behavior for `OPTIONS` preflight requests, allowed headers/methods, or future SaaS frontends on a separate origin. A library default or extension route could silently add CORS behavior.

**Fix:** State the CORS policy explicitly for community and SaaS modes, including `OPTIONS`; require opt-in allowlists for cross-origin frontends and preserve CSRF controls for cookie-authenticated requests.

### [MEDIUM] Allowed-action results are not protected from stale authorization state

**Location:** `04-tenancy-extensions.md` §5 and §6; `05-web-shell.md` §4

**Problem:** The UI consumes `actions` from API responses, while authorization can change between the read and a later mutation. The docs say the same functions calculate actions and enforce checks, but do not state that every mutation must reauthorize transactionally against current roles. A developer may treat the action flag as sufficient authorization.

**Fix:** State explicitly that action flags are advisory only and every mutation re-loads membership, roles, scopes, and safeguards inside its write transaction before changing data.

### [MEDIUM] Error and log behavior can reveal sensitive account information

**Location:** `01-foundation.md` §9–10; `03-identity-auth.md` §5 and §14; `04-tenancy-extensions.md` §5

**Problem:** The docs promise generic 404 bodies and avoid logging identifiers, but `GET /members` returns email/phone, reset metadata names the creator, setup logs a bearer token, and not-found reasons are logged. No role-based logging access, retention, redaction, or operational access controls are specified. A privacy promise without log retention/access rules is incomplete.

**Fix:** Define log access and retention, redact secrets at the logger boundary, and document the exact sensitive fields retained in database and logs, including the explicit setup-token exception.

### [MEDIUM] PostgreSQL connection pool may be exhausted by long-lived transaction callbacks

**Location:** `02-persistence.md` §2–3; `03-identity-auth.md` §3

**Problem:** `Tx.Write` callbacks must avoid network calls, but there is no transaction timeout, statement timeout, or context-cancellation requirement. Slow Argon2 work, accidental blocking, or lock waits can occupy all ten PostgreSQL connections; SQLite writer contention has a similar impact.

**Fix:** Specify transaction and statement deadlines, require context cancellation to roll back promptly, and keep password hashing and other expensive work outside write transactions unless atomicity requires otherwise.

### [MEDIUM] Argon2 semaphore is not specified as request-cancellable or process-wide

**Location:** `03-identity-auth.md` §3

**Problem:** “At most 2” computations is ambiguous across multiple server instances and says waiters time out after 10 seconds without mentioning request cancellation or queue bounds. A disconnected client can continue occupying a slot or leave waiters consuming resources.

**Fix:** Define one process-wide bounded semaphore, bounded queue behavior, request-context cancellation, and whether hashing itself can be interrupted; return `unavailable` only for a live request that exceeded the defined wait.

### [MEDIUM] SQLite and PostgreSQL tests do not cover equivalent concurrency failure modes

**Location:** `02-persistence.md` §6 and §10; `03-identity-auth.md` §13

**Problem:** PostgreSQL contract tests use template databases, while SQLite use-case tests copy a file. The listed concurrency cases cover invite limits but not one-time tokens, setup, role safeguards, session revocation, throttle increments, or migration locking. A green suite can miss the races above, especially because SQLite’s single writer masks PostgreSQL read-committed races.

**Fix:** Add cross-dialect race tests for each security-sensitive check-then-write flow, using barriers that force both transactions past the initial read before either commits.

### [MEDIUM] Partial profile updates do not define null, omission, and empty-string semantics

**Location:** `04-tenancy-extensions.md` §6; `05-web-shell.md` §4

**Problem:** PATCH payloads use optional fields, but only some fields specify whether `null` means clear, omitted means preserve, and empty string is valid. In particular email/phone cannot be changed through the listed profile API despite identifiers being editable during invite acceptance, and preference clearing semantics are unclear.

**Fix:** Define PATCH semantics per field, distinguish absent from explicit null, and state which profile identifiers may be changed and with what uniqueness/reverification flow.

### [MEDIUM] Setup token is logged as a live bearer credential

**Location:** `01-foundation.md` §9; `03-identity-auth.md` §10

**Problem:** The setup link is explicitly the only secret allowed in logs. That makes log aggregation, support bundles, container logs, or retained backups a source of a valid first-admin credential for up to 24 hours. The claim that it becomes useless after setup does not protect an uninitialized installation during that window.

**Fix:** Avoid logging the token in general application logs. Print it only to an explicitly operator-controlled setup channel, or require a separate one-time CLI retrieval mechanism with documented log access/retention controls.
