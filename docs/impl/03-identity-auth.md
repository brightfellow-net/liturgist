# 03 — Identity and Auth (Implementation)

> **Document type: Implementation.** Step 1 of [SPEC.md §10](../SPEC.md#10-suggested-build-order).
> Status: **Approved** 2026-10-02. Items marked **[P-xx]** are decisions listed in the [index](README.md#4-proposed-decisions).

## 1. Scope

Users and their identifiers, passwords, sessions, CSRF protection, login and throttling, invites, member roles and permissions, password resets, first-time setup, and the cleanup job. Tables: [reference/schema.md](../reference/schema.md). Authorization mechanics (actor, 404 vs 403, allowed actions): [04 §5](04-tenancy-extensions.md#5-authorization).

## 2. Identifiers

A user has an email, a phone number, or both ([SPEC.md §7](../SPEC.md#7-data-model-sketch)). Login accepts either in one field.

| Step | Rule |
|---|---|
| Detect | Input (after trimming spaces) contains `@` → email; otherwise → phone |
| Email | Lower-case the whole string; must parse with `net/mail.ParseAddress` as a bare address; exactly one `@`; domain part contains a `.`; ≤ 254 characters |
| Phone | Remove spaces, `-`, `(`, `)`, `.`; what remains must be digits with an optional leading `+` (the library would otherwise read letters as keypad digits); parse with `github.com/nyaruka/phonenumbers`, default region `ID`; must be `IsValidNumber`; stored in E.164 (e.g. `+6281234567890`) |
| Failure | `invalid_identifier` (422) |

- `domain.Identifier` is a value type `{Kind: Email|Phone, Value: string}` created only through `domain.ParseIdentifier`.
- Uniqueness: `users.email` and `users.phone` are each unique across the platform (`users_email_key`, `users_phone_key`).

## 3. Passwords

**[P-17]**

| Rule | Value |
|---|---|
| Normalisation | Unicode NFKC (`golang.org/x/text/unicode/norm`) before every check and before hashing; **no trimming** (spaces are part of the password) |
| Length | 10 to 128 characters (Unicode code points, after normalisation) |
| Common passwords | Rejected if, lower-cased (with or without spaces), it appears in either embedded list in `domain/commonpw`: the NCSC top 100,000 (`ncsc-100k.txt`, SecLists' `100k-most-used-passwords-NCSC.txt`, MIT licence, notice in `LICENSE-SecLists`) or the curated Indonesian and church list (`id-church.txt`, e.g. haleluya, tuhanyesus, bismillah, sayangku, rahasia, indonesia), each word also with common suffixes (`1`, `12`, `123`, `2026`, `!`, …). Only entries of at least 10 characters are kept in memory (about 9,500), since shorter passwords fail the length rule anyway |
| Own identity | Rejected if, lower-cased with spaces removed, it equals the user's email, the email's local part, the phone number with or without `+62`/`0` prefix, the user's name, or the church's name |
| Composition rules | None |
| Hash | argon2id, m = 19456 KiB, t = 2, p = 1, 16-byte salt, 32-byte key, PHC string format (`$argon2id$v=19$m=19456,t=2,p=1$<salt>$<hash>`) |
| Concurrency | One **process-wide** semaphore allows at most 2 hash computations at once (≈ 38 MiB per process; each SaaS instance has its own). At most **32 requests may wait**; a 33rd gets 503 `unavailable` immediately. A waiting request gives up when its context is cancelled (client gone) or after 10 s (`unavailable`). A computation that has started runs to completion (argon2 can't be interrupted). Hashing and verifying **never happen inside a database transaction** |
| Re-hash | On successful login, if the stored parameters differ from the current ones, re-hash and save |

Errors: `weak_password` (422) with `reason` = `too_short`, `too_long`, `common`, `matches_identity`.

## 4. Sessions

**[P-19]**

| Item | Value |
|---|---|
| Token | 32 random bytes (`crypto/rand`), base64url without padding (43 chars) |
| Stored | Only `SHA-256(token)` as hex in `sessions.token_hash` |
| Cookie name | `__Host-liturgist_session` when `BaseURL` is `https`; `liturgist_session` otherwise. If a request carries **both** names (e.g. after moving from HTTP to HTTPS), only the one for the current scheme is used, and every response that sets or clears the session cookie also clears the other name (`Max-Age=0`, same `Path=/`) |
| Cookie attributes | `HttpOnly`, `SameSite=Lax`, `Path=/`, `Secure` when `https`, no `Domain`, `Max-Age` = remaining lifetime |
| Lifetime | `SessionTTL` (default 90 days) from last use, but never more than `SessionMaxAge` (default 1 year) after `created_at`: `expires_at = min(last use + SessionTTL, created_at + SessionMaxAge)` |
| Extension | On an authenticated request, if `last_seen_at` is more than 1 hour old: one conditional update `UPDATE sessions SET last_seen_at = $now, expires_at = $new WHERE token_hash = $h AND expires_at > $now` ([02 §2.1](02-persistence.md#21-atomic-operations)) plus `users.last_seen_at = $now`. If 0 rows changed, the session was ended meanwhile: the request continues as anonymous (401 for protected operations). The cookie is re-sent only after the transaction commits. Deletion therefore always wins over extension |
| New token on every login | Login, accepting an invite, setup and password reset always create a **new** session. If the request carried a session cookie, that session is deleted first (prevents session fixation) |
| Ends | Logout deletes the current session. "Log out on all other devices" (`POST /api/v1/me/sessions/end-others`, 204) deletes all **other** sessions of that user. Changing one's password does the same. A password reset deletes **all** sessions of that user |
| User agent | First 200 characters of `User-Agent`, for a future "your devices" page; no IP address is stored |

Only the session belonging to the cookie the browser sent is deleted at login; no other sessions of either account are touched.

Session middleware: read cookie → look up hash → if missing or expired (`expires_at <= now`), treat as anonymous and clear both cookie names → else attach `app.Session{UserID, SessionID}` to the context. Set-Cookie headers are written only after the request's transactions have committed.

## 5. Login and throttling

`POST /api/v1/auth/login` `{ "identifier": string, "password": string }`

1. **Client IP and identifier keys:** determine the client address key (below) and parse the identifier. Unparseable input is not rejected early: its counters use the SHA-256 of the trimmed raw input as `H` ([schema](../reference/schema.md#auth_throttle)), so malformed input can never bypass throttling.
2. **Check all three counters** (`ip`, `idip`, `id`) in one `Tx.Read`; any locked → 429 `too_many_attempts` with `Retry-After` = the longest remaining lock.
3. **Load the user** by identifier in the same read (none for unparseable or unknown identifiers).
4. **Verify outside any transaction**, through the hash semaphore: against the user's hash, or against a fixed dummy hash when there is no user, so timing is the same.
5. **Record the result in one `Tx.Write`:**
   - failure → atomically increment all three counters ([02 §2.1](02-persistence.md#21-atomic-operations)) and return 401 `invalid_credentials` (same body whether or not the account exists);
   - success → delete the `idip` counter, delete the session from the cookie the request carried (if any), create the new session, and update the hash if it needs re-hashing (computed in step 4); after commit set the cookie and respond `204`.

**Throttling [P-18]** — table `auth_throttle`, one row per counter key:

| Counter | Key | Limit | Window | Lock |
|---|---|---|---|---|
| Identifier + IP | `idip:<H>:<A>` | 5 failures | 15 minutes from the first failure | 15 minutes |
| Identifier | `id:<H>` | 50 failures | 1 hour from the first failure | 1 hour |
| IP | `ip:<A>` | 100 failures | 15 minutes from the first failure | 15 minutes |

`H` and `A` and the exact encodings are defined in the [schema](../reference/schema.md#auth_throttle); identifiers are stored only as hashes.

**Atomic update:** each failure is one `INSERT … ON CONFLICT (key) DO UPDATE` per counter that, in SQL: starts a new window (`failures = 1`, `window_started_at = $now`) if the old window has ended; otherwise adds 1; and sets `locked_until = $now + lock` when the new count reaches the limit. When a window restarts, the lock is cleared: failures are only recorded when no counter is locked, so any old lock has already ended. Concurrent failures are never lost because the whole update is one statement.

- A login attempt is refused (429) if **any** of its three counters is locked; `Retry-After` is the longest remaining lock.
- Every failure increments all three counters. A successful login deletes the identifier+IP counter (the other two keep counting until their window ends).
- Unknown identifiers are counted the same way (keyed by the normalised input), so the response never reveals whether an account exists.
- **Client IP algorithm:**
  1. Start with the TCP remote address (port removed).
  2. If it is **not** in `LITURGIST_TRUSTED_PROXIES` ([01 §5](01-foundation.md#5-configuration)), use it. Done.
  3. If `LITURGIST_CLIENT_IP_HEADER` is set: the request must carry exactly one such header with one value that parses as an IP address (`netip.ParseAddr` after trimming spaces); use it. Otherwise fall back to the remote address.
  4. Else use `X-Forwarded-For`: join all header lines with `,`, split on `,`, trim spaces. Walk from the **right**: skip addresses inside `LITURGIST_TRUSTED_PROXIES`; the first other address is the client. If any element on that walk fails to parse (ports, `unknown`, obfuscated values), or the walk ends without a client, fall back to the remote address.
  5. Convert to the client address key `A` (IPv4 as is; IPv6 → /64 prefix).
  Fallbacks are logged at debug level only.
- **Privacy:** IP addresses exist only inside `idip` and `ip` keys. Those counters live at most 15 minutes of window plus 15 minutes of lock, and the throttle cleanup runs **every 10 minutes** ([§11](#11-cleanup-job)), so an IP address is deleted within about 40 minutes. The privacy notice says "within an hour" ([05 §2](05-web-shell.md#2-pages-in-step-1)). `id` counters hold only identifier hashes.
- **Operator recovery:** `liturgist auth clear-throttle --identifier <identifier> | --ip <address> | --all` deletes the matching counters immediately ([01 §6](01-foundation.md#6-command-line)).
- **Misconfiguration warning:** if requests arrive carrying `X-Forwarded-For` (or the configured client-IP header) while `LITURGIST_TRUSTED_PROXIES` is empty or doesn't include the sender, the server logs once per start at warn level: "Requests come through a proxy, but LITURGIST_TRUSTED_PROXIES is not set; all clients share one IP for login throttling."
- The 429 message (web app): "Too many attempts. Try again in {minutes} minutes, or ask your church admin for a reset link."

`POST /api/v1/auth/logout` → deletes the session, clears the cookie, `204`. Works without a session too (idempotent).

## 6. CSRF protection

**[P-20]** Three layers, no CSRF tokens, behind the host check of [01 §8](01-foundation.md#8-http-basics) (requests whose `Host` is not an allowed host are rejected before any of these):

1. **Cookie:** `SameSite=Lax` ([§4](#4-sessions)) — browsers don't send it on cross-site `POST`s.
2. **Cross-origin check:** Go's standard `http.CrossOriginProtection` (Go 1.25+) wraps all `/api/` routes. For `POST`, `PUT`, `PATCH`, `DELETE`: allowed if `Sec-Fetch-Site` is `same-origin` or `none`; if `Sec-Fetch-Site` is absent, allowed if `Origin` matches the request host (already restricted to allowed hosts) or a trusted origin; requests with **neither** header are allowed (non-browser clients such as curl; browsers always send at least one). The origin of `BaseURL` is registered with `AddTrustedOrigin`, so a proxy that rewrites `Host` doesn't break the check. A `same-site` request (e.g. from another brightfellow.net subdomain) is rejected.
3. **JSON only:** our own middleware rejects **every** `POST`, `PUT`, `PATCH` and `DELETE` under `/api/` whose `Content-Type` media type is not `application/json` (parameters such as `charset` allowed), **whether or not it has a body**. Bodyless operations (logout, cancel invite, regenerate, …) are sent by the web client with `Content-Type: application/json` and an empty or `{}` body. HTML forms on other sites can't send `application/json` without a CORS preflight, and the API allows no CORS ([01 §8](01-foundation.md#8-http-basics)).

Rejection by layer 2 or 3: 403 `csrf_rejected` (layer 2's deny handler is set to return our problem JSON). Requests authenticated by a bearer token (future mobile `AuthProvider`) skip layer 2; there are none in step 1.

## 7. Invites

**[P-21]** Invites need the `members.manage` scope.

| Field | Rule |
|---|---|
| `name` | 1–120 characters, trimmed |
| `email`, `phone` | At least one; each parsed as in [§2](#2-identifiers) |
| `role_ids` | IDs of roles in this church, stored in `invite_roles`; empty = team member. Rule 2 of [§8](#8-member-roles-and-permissions) applies at creation and regeneration |
| Token | 32 random bytes, base64url; only `SHA-256` stored |
| Expiry | 7 days after creation or regeneration |
| Link | `URLBuilder.AppURL("/invite") + "#t=" + token` ([01 §8](01-foundation.md#8-http-basics), [04 §4](04-tenancy-extensions.md#4-urlbuilder)) |

A **pending** invite is one with `accepted_at`, `cancelled_at` both null and `expires_at` in the future.

**Create** `POST /api/v1/invites` — inside `Tx.Write`, with `LockChurch` as the first statement:

1. Validate fields. The actor must be a member holding `members.manage` (recorded as `created_by`).
2. If a member of this church already has the email or phone → 409 `already_member`.
3. Mark expired, unaccepted, uncancelled invites for the same email or phone as cancelled. Then, if an open invite in this church has the same email or phone → 409 `invite_exists`. The partial unique indexes `invites_church_email_open_key` / `invites_church_phone_open_key` back this up; a unique violation on them also maps to `invite_exists`.
4. `Entitlements.Limit(church, MaxTeamMembers)`; if not unlimited and `memberships + pending invites ≥ limit` → 403 `limit_reached`.
5. Insert the invite and its `invite_roles` rows; respond `201` with the invite and its link. **The link is shown only in this response and in "regenerate".**

**Live roles:** an invite refers to roles, not to a frozen list of scopes. If a role is edited before acceptance, the invitee receives the role as it is at acceptance, like every existing holder of that role. This is safe because whoever edits a role must hold every scope they add ([§8](#8-member-roles-and-permissions) rule 2). Deleted roles disappear from open invites automatically (foreign key cascade).

**Regenerate** `POST /api/v1/invites/{id}/regenerate` — pending or expired (not accepted/cancelled) invites only: new token, new 7-day expiry (re-checking the limit if the invite had expired); respond with the new link. The old link stops working.

**Cancel** `DELETE /api/v1/invites/{id}` — for pending or expired invites: sets `cancelled_at`; `204`. Already accepted or cancelled invites → 404 `not_found`.

**Inspect** `POST /api/v1/invites/inspect` `{ token }` → `{ church_name, invitee_name, email, phone, status: "pending", owner_exists: bool }`, or 400 `invalid_token` with `reason`. `owner_exists` is true if the invite's email or phone belongs to an existing user (the **owner**). The invite's own email/phone are returned so the invitee can check them.

**Accept as new user** `POST /api/v1/invites/accept` `{ token, name, email?, phone?, password }`:
0. Validate and **hash the password before** opening the transaction (outside any transaction, [§3](#3-passwords)).
1. In one `Tx.Write`: **atomically claim** the invite ([02 §2.1](02-persistence.md#21-atomic-operations)); if no row was claimed, look up why and return `invalid_token` with `reason`.
2. If the invite has an owner → 409 `identifier_taken` (the page switches to "log in to accept").
3. `name`, `email`, `phone` are pre-filled from the invite in the web app and may be corrected by the invitee; at least one identifier; each parsed as in [§2](#2-identifiers). If a submitted identifier belongs to an existing user → 409 `identifier_taken`.
4. The password check of step 0 includes the submitted name and identifiers.
5. In the same transaction, create the user with the submitted details, a membership with the invite's roles (role IDs that no longer exist are skipped), mark the invite accepted (recording `accepted_user_id`), create a session; `201`.

**Accept with the logged-in account** `POST /api/v1/invites/accept-existing` `{ token }` — requires a session:
1. In one `Tx.Write`: check rule 2 below, then **atomically claim** the invite; nothing claimed → `invalid_token` with `reason`.
2. If the invite has an owner and it is **not** the logged-in user → 403 `invite_identifier_mismatch` ("This invite is for another account. Log out and log in as that person.").
3. If the invite has no owner, any logged-in user may accept (the link is the credential, as for new accounts).
4. If already a member of the church → mark the invite accepted, `200` (idempotent).
5. Otherwise create the membership with the invite's roles, mark accepted; `201`.

Both accept pages show a confirmation before the final step: "You are joining {church} as {name}". The members list shows who joined and, for open invites, who created them. An invite stays valid if the member who created it is later removed or loses `members.manage`; another admin can cancel it.

## 8. Member roles and permissions

**[P-13] [P-15] [P-16]** — model decided in [SPEC.md §4](../SPEC.md#4-users-and-roles-mvp): churches build **roles** from fixed **scopes**; members hold any number of roles; a member with no role is a **team member**. Liturgy tasks are **duties** **[P-14]** (not part of step 1).

**Scopes** (constants in `domain/scope.go`; the only permission vocabulary in code):

| Scope | Step-1 operations it allows |
|---|---|
| *(baseline, every member)* | `GET /church`, `GET/PATCH /me`, `POST /me/password` |
| `church.settings` | `PATCH /church` |
| `members.view` | `GET /members` (with email/phone) |
| `members.manage` | invites (list, create, regenerate, cancel), remove member, create reset link |
| `roles.manage` | `GET/POST/PATCH/DELETE /roles`, `PATCH /members/{id}` (assign roles) |
| `library.edit`, `templates.edit`, `liturgy.edit`, `liturgy.comment`, `liturgy.approve`, `liturgy.manage` | Defined now; first used in steps 2–5 |

`GET /roles` and `GET /scopes` (the list of scopes with translated descriptions) are allowed for `roles.manage` **or** `members.manage` (needed to pick roles when inviting).

**Ready-made roles**, created by the setup use case ([§10](#10-first-time-setup)) with `origin` set:

| `origin` | Default name (en / id) | Scopes |
|---|---|---|
| `church_admin` | Church admin / Admin gereja | `church.settings`, `members.view`, `members.manage`, `roles.manage`, `templates.edit`, `liturgy.manage` |
| `liturgist` | Liturgist / Liturgis | `members.view`, `liturgy.edit`, `liturgy.comment`, `liturgy.approve`, `liturgy.manage` |
| `editor` | Editor / Editor | `members.view`, `library.edit`, `liturgy.edit`, `liturgy.comment` |

Names are created in the church's default UI language; churches can rename them. The first admin gets the Church admin role.

**Role rules:**

| Rule | Detail |
|---|---|
| Name | 1–60 characters, trimmed; unique per church, case-insensitive → 409 `role_name_taken` |
| Description | 0–200 characters |
| Scopes | Any subset of the scope constants, including none; unknown scope → 422 `validation_failed` |
| Origin | Set only by setup and migrations; never changed through the API; kept when renamed |
| Delete | Allowed for any role, including ready-made ones; removes it from all members (the web app asks for confirmation, showing how many members hold it) |

**Safeguards** (checked inside the same `Tx.Write` as the change: the use case calls `LockChurch` first, then applies the change, then checks, then commits — so two concurrent changes in the same church are evaluated one after the other on both databases):

1. **No lock-out [P-16]:** after the change, at least one membership must hold `roles.manage` **and** `members.manage` (through any combination of roles). Otherwise → 409 `lockout_prevented`. Applies to: editing a role's scopes, deleting a role, changing a member's roles, removing a member.
2. **No escalation:** the actor must hold every scope they put into a role (create/edit), every scope of every role they assign or remove from a member, and every scope of every role in an invite they create or regenerate. Otherwise → 403 `scope_not_held` with the missing `scopes`.
3. **Removing members:** removing a member deletes the membership and its role assignments; the user account and sessions remain. The actor needs `members.manage`, plus rule 2 for the removed member's roles (you cannot remove someone more powerful than you).

**Effective scopes** of an actor = union of the scopes of their roles, computed per request in `authz.Actor` ([04 §5](04-tenancy-extensions.md#5-authorization)).

## 9. Password reset

**[P-22] [P-23]**

| Item | Rule |
|---|---|
| Token | 32 random bytes, base64url, only `SHA-256` stored |
| Lifetime | 24 hours |
| One at a time | Creating a new link takes `LockUser`, marks every unused link of that user as used (expired ones included), then inserts the new one, all in one `Tx.Write`; the partial unique index `password_resets_user_open_key` backs this up |
| Link | `URLBuilder.AppURL("/reset") + "#t=" + token` |

**Admin-created** `POST /api/v1/members/{membershipId}/password-reset` (`members.manage`, plus [§8](#8-member-roles-and-permissions) rule 2 for the member's roles):
- If the user has a membership in any other church → 409 `reset_not_allowed` (prevents an admin of one church taking over an account used in another church on the SaaS).
- Respond `201 { link, expires_at }`.
- `GET /api/v1/members` includes, for viewers with `members.manage`, each member's latest reset link: `last_reset: { created_by_name, created_at, expires_at, used_at } | null` (never the link itself).
- Log lines at info: `reset_link_created` (`actor` = user ID, or `cli` for the command line; target user ID) and `reset_link_used` (user ID).

**CLI** `liturgist user reset-password <identifier>`: prints the user's name, the link and its expiry (in the church's time zone) to stdout. Never accepts a password argument. No church or scope check (the operator controls the server). The reset page shows "created by the server administrator". If `LITURGIST_BASE_URL` is not set in the environment, prints a warning that the link may point to `localhost`. The identifier is parsed with `domain.ParseIdentifier`; emails and phone numbers are unique across the platform, so it matches at most one user. Logged as `reset_link_created` with `actor=cli`. Unknown identifier → exit 5. Works while the server is running (SQLite WAL), e.g. `docker exec <container> liturgist user reset-password …`.

**Recovery CLI** (operator only): `liturgist user list` prints names, identifiers and roles; `liturgist member grant-admin <identifier>` gives the member the ready-made Church admin role, recreating that role with its default scopes if it was deleted. For when no reachable member holds `roles.manage` and `members.manage` (e.g. the only admin left the church).

**Inspect** `POST /api/v1/auth/reset/inspect` `{ token }` → `{ user_name, created_by_name | null, expires_at }` or 400 `invalid_token`; the reset page shows "This link was created by {created_by_name}" (or "by the server administrator" for CLI links).

**Use** `POST /api/v1/auth/reset` `{ token, new_password }`:
1. Look up the token's user (`Tx.Read`) to validate the password against their identity, then validate and **hash** the new password outside any transaction.
2. In one `Tx.Write`: **atomically claim** the token (`UPDATE password_resets SET used_at = $now WHERE token_hash = $h AND used_at IS NULL AND expires_at > $now`); 0 rows → `invalid_token` with `reason`. Then save the new hash, delete all the user's sessions, delete the user's `id` and `idip` throttle counters, and create a new session.
3. After commit set the cookie; `204`. Of two simultaneous uses, exactly one succeeds.

**Change own password** `POST /api/v1/me/password` `{ current_password, new_password }`: verify the current password and hash the new one outside any transaction (a wrong current password counts toward throttling as a login failure → `invalid_credentials`); then in one `Tx.Write` save the hash and delete all **other** sessions; `204`.

Email-based reset is not part of step 1 (no email support yet).

## 10. First-time setup

**[P-24]**

**Setup token:** stored only as `SHA-256` in the singleton `setup_tokens` row ([schema](../reference/schema.md#setup_tokens)), valid **24 hours**. Issuing a token is one upsert on that row under `LockInstall`, so concurrent issuers (two instances starting, or a start plus `setup-link`) leave exactly one valid token, the last one written.

- At `serve` start, if `Churches().Count() == 0`: create a new token and print the link as a framed block to stderr **and** log it once at warn level:

  ```
  ================================================================
    Liturgist is not set up yet.
    Open this link to create your church (valid 24 hours):
    http://localhost:8080/setup#t=<token>
  ================================================================
  ```

- Each new token replaces the previous one, so **the most recently printed link is the only valid one**. With several instances against one PostgreSQL database (not a community setup), use `liturgist setup-link` to get a link that is valid at that moment. After a restart or crash, use the link printed by the latest start, or run `liturgist setup-link`.
- `liturgist setup-link` does the same on demand (e.g. `docker exec <container> liturgist setup-link`); exit 4 if already set up. It warns if `LITURGIST_BASE_URL` is unset, like the reset command.
- There is no exception for requests from `localhost`: behind a reverse proxy every request looks local.
- **Accepted risk (review round 2):** the setup link is a live credential in the server log and stderr until setup or expiry (24 h). Accepted by the owner because no church data exists before setup, people who can read container logs usually control the host, and the link stops working once used, replaced or expired. It is the only secret the logger's redaction allows through ([01 §9](01-foundation.md#9-logging)).
- `GET /api/v1/setup/status` → `{ "set_up": bool }` (public). The web app's `/setup` page shows "Already set up — go to login" when `set_up` is true.

**`POST /api/v1/setup`** (public, CSRF rules apply):

```json
{
  "token": "…",
  "church": {
    "name": "GKY Citragarden",
    "default_ui_language": "en",
    "default_language": "id",
    "default_translation_code": "TB",
    "time_zone": "Asia/Jakarta",
    "key_display": "do"
  },
  "admin": { "name": "…", "identifier": "…", "password": "…" }
}
```

| Field | Rule |
|---|---|
| `name` | 1–120 characters |
| `default_ui_language` | `en` or `id` |
| `default_language` | `id`, `en`, `zh-Hans`, `zh-Hant` |
| `default_translation_code` | A code in `translations` |
| `time_zone` | Loadable by `time.LoadLocation`; the wizard offers `Asia/Jakarta` (WIB, pre-selected), `Asia/Makassar` (WITA), `Asia/Jayapura` (WIT) |
| `key_display` | `do` ("Do = G") or `letter` ("G") |

Validate fields and hash the admin password outside any transaction. Then in one `Tx.Write`: take `LockInstall`; **atomically claim** the token (`DELETE FROM setup_tokens WHERE id = 1 AND token_hash = $h AND expires_at > $now`, 1 row required, otherwise 400 `invalid_token`); if any church exists → 409 `already_set_up`; create the church, the three ready-made roles ([§8](#8-member-roles-and-permissions)), the user, and a membership holding the Church admin role; create a session; respond `201` after commit. The resolver cache is refreshed ([04 §3](04-tenancy-extensions.md#3-tenantresolver)).

**CLI** `liturgist setup --church-name … --admin-name … --admin-identifier … [--ui-language en] [--language id] [--translation TB] [--time-zone Asia/Jakarta] [--key-display do]`. The password is read from the terminal without echo, or from stdin with `--password-stdin`. Calls the same use case without a token, but under the same `LockInstall` and church-count check, so the CLI and the web wizard can never both create a church. Already set up → exit 4.

The "regular services" wizard step ([SPEC.md §5.7](../SPEC.md#57-first-time-experience)) is added with services in step 3.

**Defaults for later features:** setup in step 1 creates only the ready-made roles. Each later step that introduces seeded defaults (duties, singing parts, starter template) ships a migration that also seeds them into every **existing** church that has none yet, in that church's default content or UI language as the feature requires. New churches get them from setup.

## 11. Cleanup job

**[P-32]** Expiry is decided **at use time**: every token, session or throttle check compares `expires_at` / `locked_until` with `Clock.Now()` (UTC) inside the same transaction that uses it. The cleanup job is storage housekeeping only; a row it hasn't deleted yet is still treated as expired.

A goroutine started by `serve` runs one minute after start, then: **throttle rows every 10 minutes**, everything else hourly; each run in its own `Tx.Write`:

| Deletes | Condition |
|---|---|
| Sessions | `expires_at < now` |
| Password resets | `expires_at < now - 7 days` or `used_at < now - 7 days` |
| Invites | Never deleted (history); expired ones simply stop counting |
| Throttle rows | Window ended and lock (if any) ended |
| Setup tokens | `expires_at < now` |

## 12. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Store session, invite, reset or setup tokens in plain text | Store `SHA-256(token)` only | A database leak must not give working links |
| Return different errors or timings for "unknown user" and "wrong password" | Same `invalid_credentials`, dummy hash for unknown users | Prevents discovering who has an account |
| Accept a password on the command line | Prompt without echo or `--password-stdin` | Shell history and process lists |
| Compare tokens or hashes with `==` on secrets | Look up by hash; `subtle.ConstantTimeCompare` where comparing | Timing attacks |
| Let a church admin reset the password of a user who belongs to another church | `reset_not_allowed` | Account takeover across churches on the SaaS |
| Normalise identifiers in more than one place | `domain.ParseIdentifier` only | Duplicate accounts from inconsistent normalisation |
| Use form-encoded bodies, skip the cross-origin check, or write a custom origin check | JSON bodies + Go's `http.CrossOriginProtection` | CSRF; the standard library's check is maintained and reviewed |
| Compute argon2 hashes without the semaphore, or inside a transaction | Go through the password hasher, before or after the transaction | Memory exhaustion on 512 MB servers; long transactions block the database |
| Check that a token is unused with `SELECT`, then mark it used later | Atomic claim ([02 §2.1](02-persistence.md#21-atomic-operations)) | Two simultaneous uses would both succeed on PostgreSQL |
| Check throttling only after parsing the identifier | IP counter first ([§5](#5-login-and-throttling)) | Malformed input would bypass throttling and burn hash capacity |
| Log identifiers, names or tokens (except the setup link) | Log user IDs only; the logger redacts known secret keys ([01 §9](01-foundation.md#9-logging)) | UU PDP; secrets |

## 13. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-A-001 | `ParseIdentifier` | `0812-3456-7890` | Phone `+6281234567890` | `+62 812…`, `62812…`, too short → `invalid_identifier` |
| TC-A-002 | `ParseIdentifier` | ` Budi@Example.ORG ` | Email `budi@example.org` | Two `@`; no dot in domain; 255 chars |
| TC-A-003 | Password policy | `password123` | `weak_password`/`common` | 9 chars → `too_short`; 129 → `too_long`; equals email local part, own name or church name (case and spaces ignored) → `matches_identity`; `Haleluya2026` → `common`; 10 emoji code points accepted; trailing space kept and must match at login; `é` typed as one or two code points verifies the same |
| TC-A-004 | Hasher | Hash then verify | Verifies; wrong password fails | Old parameters → `NeedsRehash` true |
| TC-A-005 | Throttle | 5 failures for one identifier from one IP within 15 min | 6th attempt from that IP → 429, `Retry-After` ≈ 15 min; same identifier from another IP → still allowed | 50 failures for the identifier across IPs within 1 h → locked everywhere for 1 h; 100 failures from one IP across identifiers → IP locked; failures 16 min apart don't lock |
| TC-A-011 | Client IP | Remote `127.0.0.1` (trusted), `X-Forwarded-For: 203.0.113.5, 127.0.0.1` | `203.0.113.5` | Remote untrusted → header ignored; `LITURGIST_CLIENT_IP_HEADER=CF-Connecting-IP` from trusted remote → header value; IPv6 → /64 prefix |
| TC-A-006 | CSRF check | `POST` with `Sec-Fetch-Site: cross-site` | 403 `csrf_rejected` | `same-site` → rejected; no `Sec-Fetch-Site` with matching `Origin` → allowed; `Origin` equal to `BaseURL` while `Host` differs (proxy) → allowed; neither header → allowed; `text/plain` body → rejected; bodyless `POST` without `Content-Type` → rejected; bodyless `POST` with `application/json` → allowed |
| TC-A-007 | Lock-out rule | Remove `roles.manage` from the only role holding it | `lockout_prevented` | Another member still holds both scopes through a custom role → allowed |
| TC-A-009 | Escalation rule | Actor with `members.manage` but not `liturgy.approve` invites with the Liturgist role | 403 `scope_not_held` listing `liturgy.approve`, `liturgy.edit`, … | Actor holds all scopes → allowed |
| TC-A-010 | Effective scopes | Member with Editor + custom role {`church.settings`} | Union of both scope sets | No roles → empty (baseline only) |
| TC-A-008 | Invite status | Invite with `expires_at` in the past | Not pending; `invalid_token`/`expired` on accept | Cancelled → `cancelled`; accepted → `used` |

### Integration tests (use cases on temp-file SQLite; HTTP tests through `httptest`)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-A-001 | Setup | Fresh install | Status `set_up:false`; setup with token → 201, cookie set, three ready-made roles exist; second setup → 409 `already_set_up`; wrong or 25-hour-old token → `invalid_token`; `setup-link` invalidates the token printed at start | Temp dir removed |
| IT-A-002 | Login | User with known password | Correct → 204 + cookie; wrong → 401; 6th failure → 429; unknown identifier → 401 with same body | — |
| IT-A-003 | Session lifetime | Session with `last_seen_at` 2 h ago | Request extends `expires_at`; expired session → anonymous and cookie cleared | Session created 364 days ago and used daily → `expires_at` capped at created + 1 year |
| IT-A-011 | Session fixation and ending others | Browser with an existing session logs in as another user; user with 3 sessions | Old session row deleted, new token issued; `end-others` leaves only the current session | — |
| IT-A-004 | Invite new user | Admin creates invite with a mistyped phone | Inspect → `owner_exists:false` and the invite's phone; accept with a corrected phone → user has the corrected phone, membership with roles, session; link reused → `invalid_token`/`used` | Corrected email belongs to another user → `identifier_taken` |
| IT-A-005 | Invite existing user | User U exists with phone P; invite for P | Accept while logged in as U → membership; as another user → 403 `invite_identifier_mismatch`; accept as new → 409 `identifier_taken` | Invite for an unused phone, accepted by logged-in user V (account under email only) → V becomes a member |
| IT-A-006 | Invite limit | Entitlements stub returning limit 2; 1 member + 1 pending invite | Third invite → 403 `limit_reached`; cancel one → invite allowed | — |
| IT-A-007 | Concurrent invites at limit | Limit 2, 1 member; two invite requests in parallel | Exactly one succeeds (both dialects) | — |
| IT-A-008 | Admin reset | Member M | Inspect shows the admin's name; link works once; all M's sessions deleted; `GET /members` shows `last_reset` with `used_at`; M also member elsewhere → `reset_not_allowed` | Admin lacking a scope of M's roles → 403 `scope_not_held`; second link cancels the first |
| IT-A-012 | CLI recovery | Only admin unreachable; member M | `member grant-admin M` → M holds Church admin; with the role deleted beforehand → role recreated with default scopes; `user reset-password` without base URL → warning on stderr | — |
| IT-A-009 | Change own password | Logged in on two sessions | Other session invalid afterwards; current remains | — |
| IT-A-010 | CSRF over HTTP | Cross-site `POST /api/v1/auth/logout` | 403 `csrf_rejected` | — |
| IT-A-013 | Races (both dialects, barrier after the first read) | One invite, one reset link, one setup token; two requests each | Exactly one acceptance / reset / setup succeeds; the other gets `invalid_token`; exactly one church and one membership exist | Two simultaneous reset-link creations → one open link; session extension racing with logout → session stays deleted, no cookie re-issued |
| IT-A-014 | Throttle counters under concurrency | 10 parallel failures for one identifier+IP | `failures = 10`, lock set at the 5th; no lost increments (both dialects) | Malformed identifiers counted against the IP |
| IT-A-015 | Hash queue | 2 hashes running, 32 waiting | 35th request → immediate 503; a waiting request whose client disconnects leaves the queue | — |
| IT-A-016 | Cookie names | Request with both cookie names | Current-scheme cookie used; response clears the other | — |

## 14. Error handling matrix

| Error | Detection | Response | Fallback | Logging |
|---|---|---|---|---|
| Wrong password / unknown identifier | Verify fails / no user | 401 `invalid_credentials` | — | info: `login_failed`, user ID if known |
| Throttled | Any of the three counters locked | 429 `too_many_attempts` + `Retry-After` | Wait, or ask for a reset link | warn: counter type (`idip`/`id`/`ip`), not the value |
| Hash semaphore timeout or full queue | 10 s wait, or 32 waiting | 503 `unavailable` | Retry | warn |
| Token claimed by a concurrent request | Atomic claim changed 0 rows | 400 `invalid_token` (`used`) | — | info |
| Token unknown/expired/used/cancelled | Lookup by hash | 400 `invalid_token` + `reason` | Ask admin for a new link | info |
| Identifier already used | Unique violation `users_email_key`/`users_phone_key` | 409 `identifier_taken` | Log in instead | info |
| Lock-out | Safeguard 1 | 409 `lockout_prevented` | Give someone else the scopes first | info |
| Escalation | Safeguard 2 | 403 `scope_not_held` + `scopes` | Ask someone who holds them | info |
| Setup while set up | Church count > 0 | 409 / exit 4 | — | info |
| CSRF rejected | Middleware | 403 `csrf_rejected` | — | warn with `Sec-Fetch-Site` and origin host |

## 15. References

| Topic | Location |
|---|---|
| Auth decisions (invite-only, identifiers, reset, setup, sessions) | [SPEC.md §11](../SPEC.md#11-decisions-log) — rows "Auth for MVP", "Login identifier", "Password reset", "Users are platform-wide", "First-time setup", "Sessions last 90 days" |
| Team-member limit rules | [SPEC.md §8.2.1](../SPEC.md#821-usage-limits-saas-free-plan) |
| Setup wizard | [SPEC.md §5.7](../SPEC.md#57-first-time-experience) |
| Roles, scopes, safeguards | [SPEC.md §4](../SPEC.md#4-users-and-roles-mvp) |
| Tables | [reference/schema.md](../reference/schema.md) |
| Authorization, 404 vs 403, allowed actions | [04-tenancy-extensions.md §5](04-tenancy-extensions.md#5-authorization) |
| Error codes | [01-foundation.md §10](01-foundation.md#10-error-format-and-codes) |
