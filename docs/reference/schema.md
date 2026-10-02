# Database Schema (Reference)

> **Document type: Reference.** Tables that exist after each build step. The step-1 tables are: translations, churches, users, memberships, roles, role_scopes, membership_roles, invites, invite_roles, sessions, password_resets, auth_throttle, setup_tokens. Conceptual model: [SPEC.md §7](../SPEC.md#7-data-model-sketch). Type mapping, migrations and error mapping: [02-persistence.md](../impl/02-persistence.md).
> Status: **Approved** 2026-10-02 for step 1, including the round-2 review fixes. Proposal markers refer to the [index](../impl/README.md#4-proposed-decisions).

## Conventions

- Types below are logical; physical types per database follow [02 §4](../impl/02-persistence.md#4-type-mapping): `id` = ULID text (26 chars), `ts` = UTC timestamp at microsecond precision, `json` = JSON value, `bool`, `text`, `int`.
- SQLite tables are `STRICT`. Every constraint and index name below is identical in both migrations.
- **Enforcement marker:** a rule written as SQL is enforced by the database; the SQL is given per dialect where it differs. A rule marked **(app)** is enforced only by the application, through the `domain` constructors used by every write path, and covered by a repository test that attempts a non-conforming write. Case rules are app-only because Unicode case folding differs between Go, SQLite and PostgreSQL.
- **Platform tables** have no `church_id`. **Church-owned tables** have `church_id`, composite foreign keys including `church_id` ([SPEC.md §8.1](../SPEC.md#81-tenancy) rule 1), and a **church-leading index**: an index, primary key or unique constraint whose first column is `church_id`. A table without one must list the exception and its reason here (none in step 1).
- No database defaults for IDs or timestamps.
- **Token hashes** (`token_hash` columns) are 64 lower-case hex characters: SQLite `CHECK (length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*')`; PostgreSQL `CHECK (token_hash ~ '^[0-9a-f]{64}$')`. Check names: `<table>_token_hash_check`.
- **Time order:** where a table has both, `expires_at > created_at` (`<table>_expiry_check`). Fixed-width timestamp text makes this comparison correct in SQLite.
- **JSON columns:** the adapter validates the shape on every write (Go structs); SQLite additionally checks `json_valid`. Unknown keys in objects are preserved. PostgreSQL `jsonb` may reorder keys; equality is semantic, never by bytes.

## Step 1 tables

### translations

Platform table. Seeded by migration with fixed IDs **[P-31]**.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `translations_pkey` |
| `code` | text | no | UNIQUE `translations_code_key`; upper-case letters and digits, 1–16 chars (app) |
| `name` | text | no | |
| `language` | text | no | `CHECK (language IN ('id','en','zh-Hans','zh-Hant'))` `translations_language_check` |

Seed rows (IDs are fixed ULIDs generated once and written into both migrations):

| code | name | language |
|---|---|---|
| TB | Terjemahan Baru (LAI) | id |
| TB2 | Terjemahan Baru Edisi Kedua (LAI) | id |
| BIS | Bahasa Indonesia Sehari-hari (LAI) | id |
| CUV | 和合本 (Chinese Union Version) | zh-Hans |
| KJV | King James Version | en |
| WEB | World English Bible | en |

### churches

Platform-level row that owns all church data. The community "at most one church" rule is enforced by the setup use case under `LockInstall` ([03 §10](../impl/03-identity-auth.md#10-first-time-setup)) and checked at start-up ([04 §3](../impl/04-tenancy-extensions.md#3-tenantresolver)); it is not a table constraint, because the SaaS uses the same schema for many churches.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `churches_pkey` |
| `name` | text | no | 1–120 chars (app) |
| `default_ui_language` | text | no | `CHECK (… IN ('en','id'))` `churches_ui_language_check` |
| `default_language` | text | no | `CHECK (… IN ('id','en','zh-Hans','zh-Hant'))` `churches_language_check` |
| `default_translation_id` | id | **no** | FK → `translations(id)` `churches_translation_fkey`. Independent of `default_language` by design (e.g. a church may default to TB while running English UI); the setup wizard only pre-selects a matching translation |
| `time_zone` | text | no | IANA name (app) |
| `settings` | json | no | Object. Keys: `key_display` (`do`\|`letter`), `feedback_url`, `privacy_contact`; unknown keys preserved |
| `created_at` | ts | no | |
| `updated_at` | ts | no | |

### users

Platform table.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `users_pkey` |
| `name` | text | no | 1–120 chars (app) |
| `email` | text | yes | UNIQUE `users_email_key`; lower-case (app) |
| `phone` | text | yes | UNIQUE `users_phone_key`; E.164: SQLite `CHECK (phone GLOB '+[1-9]*' AND phone NOT GLOB '+*[^0-9]*' AND length(phone) BETWEEN 8 AND 16)`, PostgreSQL `CHECK (phone ~ '^\+[1-9][0-9]{6,14}$')` `users_phone_check` |
| `password_hash` | text | no | PHC string (app) |
| `preferences` | json | no | Object. Keys: `text_size` (`normal`\|`large`\|`larger`), `ui_language` (`en`\|`id`\|absent) |
| `last_seen_at` | ts | yes | |
| `created_at` | ts | no | |
| `updated_at` | ts | no | |

Table check `users_identifier_check`: `email IS NOT NULL OR phone IS NOT NULL`.

### memberships

Church-owned.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `memberships_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `memberships_church_fkey` |
| `user_id` | id | no | FK → `users(id)` ON DELETE CASCADE `memberships_user_fkey` |
| `created_at` | ts | no | |

- UNIQUE `memberships_church_user_key` (`church_id`, `user_id`) — church-leading.
- UNIQUE `memberships_church_id_key` (`church_id`, `id`) — target for composite foreign keys.
- Index `memberships_user_idx` (`user_id`) — for "memberships of this user" (platform path, [03 §9](../impl/03-identity-auth.md#9-password-reset)).

### roles

Church-owned. **[P-13]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `roles_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `roles_church_fkey` |
| `name` | text | no | 1–60 chars (app) |
| `name_key` | text | no | Lower-cased, trimmed `name` (app); UNIQUE `roles_church_name_key` (`church_id`, `name_key`) — church-leading |
| `description` | text | no | 0–200 chars (app); empty string allowed |
| `origin` | text | yes | `CHECK (origin IN ('church_admin','liturgist','editor'))` `roles_origin_check`; UNIQUE `roles_church_origin_key` (`church_id`, `origin`) — at most one of each ready-made role |
| `created_at` | ts | no | |
| `updated_at` | ts | no | |

- UNIQUE `roles_church_id_key` (`church_id`, `id`) — target for composite foreign keys.

### role_scopes

Church-owned.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `church_id` | id | no | |
| `role_id` | id | no | |
| `scope` | text | no | Validated on write against the constants in `domain/scope.go` (app). No `CHECK`, so adding a scope needs no table rebuild. **Unknown stored scopes grant nothing**: when roles are loaded, unknown values are dropped and logged once per process at warn level |

- PK `role_scopes_pkey` (`role_id`, `scope`).
- Index `role_scopes_church_role_idx` (`church_id`, `role_id`) — church-leading.
- FK `role_scopes_role_fkey` (`church_id`, `role_id`) → `roles(church_id, id)` ON DELETE CASCADE.

### membership_roles

Church-owned.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `church_id` | id | no | |
| `membership_id` | id | no | |
| `role_id` | id | no | |

- PK `membership_roles_pkey` (`membership_id`, `role_id`).
- FK `membership_roles_membership_fkey` (`church_id`, `membership_id`) → `memberships(church_id, id)` ON DELETE CASCADE.
- FK `membership_roles_role_fkey` (`church_id`, `role_id`) → `roles(church_id, id)` ON DELETE CASCADE.
- Index `membership_roles_role_idx` (`church_id`, `role_id`) — church-leading; member counts per role and the lock-out check.

### invites

Church-owned. **[P-21]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `invites_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `invites_church_fkey` |
| `token_hash` | text | no | UNIQUE `invites_token_hash_key`; token-hash check |
| `name` | text | no | 1–120 chars (app) |
| `email` | text | yes | lower-case (app) |
| `phone` | text | yes | E.164, same check as `users.phone` (`invites_phone_check`) |
| `created_by` | id | yes | FK → `users(id)` ON DELETE SET NULL `invites_created_by_fkey`. **Audit label:** the inviting user, who must be a member with `members.manage` at creation time (app); kept after that user leaves the church |
| `created_at` | ts | no | |
| `expires_at` | ts | no | expiry check |
| `accepted_at` | ts | yes | |
| `accepted_user_id` | id | yes | FK → `users(id)` ON DELETE SET NULL `invites_accepted_user_fkey` |
| `cancelled_at` | ts | yes | |

- UNIQUE `invites_church_id_key` (`church_id`, `id`) — target for composite foreign keys.
- `invites_identifier_check`: `email IS NOT NULL OR phone IS NOT NULL`.
- `invites_state_check`: `NOT (accepted_at IS NOT NULL AND cancelled_at IS NOT NULL)`.
- `invites_accepted_user_check`: `accepted_user_id IS NULL OR accepted_at IS NOT NULL` (the user ID may become null later only through `ON DELETE SET NULL`).
- Index `invites_church_open_idx` (`church_id`, `expires_at`) WHERE `accepted_at IS NULL AND cancelled_at IS NULL` — church-leading.
- **One open invite per identifier:** UNIQUE `invites_church_email_open_key` (`church_id`, `email`) WHERE `email IS NOT NULL AND accepted_at IS NULL AND cancelled_at IS NULL`; UNIQUE `invites_church_phone_open_key` (`church_id`, `phone`) with the same condition. Expired-but-open invites still occupy the slot, so invite creation cancels them first ([03 §7](../impl/03-identity-auth.md#7-invites)).

### invite_roles

Church-owned. Replaces the earlier `invites.role_ids` JSON column.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `church_id` | id | no | |
| `invite_id` | id | no | |
| `role_id` | id | no | |

- PK `invite_roles_pkey` (`invite_id`, `role_id`).
- FK `invite_roles_invite_fkey` (`church_id`, `invite_id`) → `invites(church_id, id)` ON DELETE CASCADE.
- FK `invite_roles_role_fkey` (`church_id`, `role_id`) → `roles(church_id, id)` ON DELETE CASCADE — a deleted role disappears from open invites automatically; roles of another church cannot be referenced.
- Index `invite_roles_church_role_idx` (`church_id`, `role_id`) — church-leading.
- Invites point to **live** roles: if a role's scopes change before acceptance, the invitee receives the role as it is at acceptance, exactly like existing holders of that role ([03 §7](../impl/03-identity-auth.md#7-invites)).

### sessions

Platform table. **[P-19]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `token_hash` | text | no | PK `sessions_pkey`; token-hash check |
| `user_id` | id | no | FK → `users(id)` ON DELETE CASCADE `sessions_user_fkey` |
| `user_agent` | text | no | ≤ 200 chars (app); empty string allowed |
| `created_at` | ts | no | |
| `last_seen_at` | ts | no | `sessions_seen_check`: `last_seen_at >= created_at` |
| `expires_at` | ts | no | expiry check |

Indexes: `sessions_user_idx` (`user_id`), `sessions_expires_idx` (`expires_at`).

### password_resets

Platform table. **[P-22]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `password_resets_pkey` |
| `user_id` | id | no | FK → `users(id)` ON DELETE CASCADE `password_resets_user_fkey` |
| `token_hash` | text | no | UNIQUE `password_resets_token_hash_key`; token-hash check |
| `created_by` | id | yes | FK → `users(id)` ON DELETE SET NULL; null when created by the CLI |
| `created_at` | ts | no | |
| `expires_at` | ts | no | expiry check |
| `used_at` | ts | yes | |

- Index `password_resets_user_idx` (`user_id`).
- **At most one unused link per user:** UNIQUE `password_resets_user_open_key` (`user_id`) WHERE `used_at IS NULL`. Creating a link first marks every unused link of that user as used (expired ones included), under `LockUser` ([03 §9](../impl/03-identity-auth.md#9-password-reset)).

### auth_throttle

Platform table. **[P-18]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `key` | text | no | PK `auth_throttle_pkey`. Canonical encodings below |
| `kind` | text | no | `CHECK (kind IN ('idip','id','ip'))` `auth_throttle_kind_check` |
| `failures` | int | no | `CHECK (failures >= 0)` `auth_throttle_failures_check` |
| `window_started_at` | ts | no | |
| `locked_until` | ts | yes | `auth_throttle_lock_check`: `locked_until IS NULL OR locked_until >= window_started_at` |

**Key encodings** (collision-free: every variable part has a fixed alphabet and length or is the last part):

| Kind | Key | Parts |
|---|---|---|
| `idip` | `idip:<H>:<A>` | `H` = lower-case hex SHA-256 (64 chars) of the normalised identifier ([03 §2](../impl/03-identity-auth.md#2-identifiers)), or of the trimmed raw input for unparseable identifiers; `A` = client address key |
| `id` | `id:<H>` | as above |
| `ip` | `ip:<A>` | client address key |

Client address key `A`: IPv4 in dotted decimal (e.g. `203.0.113.5`); IPv6 as the /64 prefix in canonical compressed form with prefix length (e.g. `2001:db8:1:2::/64`). Identifiers are never stored in plain text in this table.

### setup_tokens

Platform table. **[P-24]** Exactly zero or one row, enforced by the database.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | int | no | PK `setup_tokens_pkey`; `CHECK (id = 1)` `setup_tokens_singleton_check` |
| `token_hash` | text | no | token-hash check |
| `created_at` | ts | no | |
| `expires_at` | ts | no | expiry check |

Issuing a token is one upsert on `id = 1` (`INSERT … ON CONFLICT (id) DO UPDATE`), so concurrent issuers leave exactly one valid token: the last writer's.

## Later steps

Tables for songs, readings, templates, services, liturgies, comments, edits, published versions and imports are added in the steps that build them, following [SPEC.md §7](../SPEC.md#7-data-model-sketch) and the conventions above.

## References

| Topic | Location |
|---|---|
| Conceptual data model | [SPEC.md §7](../SPEC.md#7-data-model-sketch) |
| Type mapping, migrations, error mapping, atomic operations | [02-persistence.md](../impl/02-persistence.md) |
| Identity and auth rules using these tables | [03-identity-auth.md](../impl/03-identity-auth.md) |
| Tenancy rules | [SPEC.md §8.1](../SPEC.md#81-tenancy) |
| Review findings this version answers | [schema-adversarial-review.md](schema-adversarial-review.md), [review log](../impl/README.md#6-review-log) |
