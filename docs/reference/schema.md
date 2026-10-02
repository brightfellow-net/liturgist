# Database Schema (Reference)

> **Document type: Reference.** Tables that exist after each build step. The step-1 tables are: translations, churches, users, memberships, roles, role_scopes, membership_roles, invites, sessions, password_resets, auth_throttle, setup_tokens. Conceptual model: [SPEC.md §7](../SPEC.md#7-data-model-sketch). Type mapping, migrations and error mapping: [02-persistence.md](../impl/02-persistence.md).
> Status: **Draft** for step 1. Proposal markers refer to the [index](../impl/README.md#3-proposed-decisions).

## Conventions

- Types below are logical; physical types per database follow [02 §4](../impl/02-persistence.md#4-type-mapping): `id` = ULID text (26 chars), `ts` = UTC timestamp, `json` = JSON object, `bool`, `text`, `int`.
- SQLite tables are `STRICT`. Every constraint name below is identical in both migrations.
- **Platform tables** have no `church_id`. **Church-owned tables** have `church_id`, an index starting with `church_id`, and composite foreign keys including `church_id` ([SPEC.md §8.1](../SPEC.md#81-tenancy) rule 1).
- No database defaults for IDs or timestamps.

## Step 1 tables

### translations

Platform table. Seeded by migration with fixed IDs **[P-31]**.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `translations_pkey` |
| `code` | text | no | UNIQUE `translations_code_key`; upper-case letters and digits, 1–16 chars (validated in app) |
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

Platform-level row that owns all church data.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `churches_pkey` |
| `name` | text | no | 1–120 chars (app) |
| `default_ui_language` | text | no | `CHECK (… IN ('en','id'))` `churches_ui_language_check` |
| `default_language` | text | no | `CHECK (… IN ('id','en','zh-Hans','zh-Hant'))` `churches_language_check` |
| `default_translation_id` | id | yes | FK → `translations(id)` `churches_translation_fkey` |
| `time_zone` | text | no | IANA name (app) |
| `settings` | json | no | Keys: `key_display` (`do`\|`letter`), `feedback_url`, `privacy_contact`; unknown keys preserved |
| `created_at` | ts | no | |
| `updated_at` | ts | no | |

### users

Platform table.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `users_pkey` |
| `name` | text | no | 1–120 chars (app) |
| `email` | text | yes | UNIQUE `users_email_key`; lower-case (app) |
| `phone` | text | yes | UNIQUE `users_phone_key`; E.164 (app) |
| `password_hash` | text | no | PHC string |
| `preferences` | json | no | Keys: `text_size` (`normal`\|`large`\|`larger`), `ui_language` (`en`\|`id`\|absent) |
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

- UNIQUE `memberships_church_user_key` (`church_id`, `user_id`).
- UNIQUE `memberships_church_id_key` (`church_id`, `id`) — target for composite foreign keys.
- Index `memberships_user_idx` (`user_id`) — for "memberships of this user" (platform path, [03 §9](../impl/03-identity-auth.md#9-password-reset)).

### roles

Church-owned. **[P-13]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `roles_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `roles_church_fkey` |
| `name` | text | no | 1–60 chars (app) |
| `name_key` | text | no | Lower-cased, trimmed `name`; UNIQUE `roles_church_name_key` (`church_id`, `name_key`) |
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
| `scope` | text | no | Validated in app against the constants in `domain/scope.go` (no `CHECK`, so adding a scope needs no table rebuild) |

- PK `role_scopes_pkey` (`role_id`, `scope`).
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
- Index `membership_roles_role_idx` (`church_id`, `role_id`) — member counts per role and the lock-out check.

### invites

Church-owned. **[P-21]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `invites_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `invites_church_fkey` |
| `token_hash` | text | no | UNIQUE `invites_token_hash_key` |
| `name` | text | no | |
| `email` | text | yes | |
| `phone` | text | yes | |
| `role_ids` | json | no | JSON array of role IDs at the time of inviting; IDs deleted before acceptance are skipped |
| `created_by` | id | yes | FK → `users(id)` ON DELETE SET NULL `invites_created_by_fkey` |
| `created_at` | ts | no | |
| `expires_at` | ts | no | |
| `accepted_at` | ts | yes | |
| `accepted_user_id` | id | yes | FK → `users(id)` ON DELETE SET NULL `invites_accepted_user_fkey` |
| `cancelled_at` | ts | yes | |

- Table check `invites_identifier_check`: `email IS NOT NULL OR phone IS NOT NULL`.
- Index `invites_church_open_idx` (`church_id`, `expires_at`) WHERE `accepted_at IS NULL AND cancelled_at IS NULL`.

### sessions

Platform table. **[P-19]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `token_hash` | text | no | PK `sessions_pkey` |
| `user_id` | id | no | FK → `users(id)` ON DELETE CASCADE `sessions_user_fkey` |
| `user_agent` | text | no | ≤ 200 chars (app); empty string allowed |
| `created_at` | ts | no | |
| `last_seen_at` | ts | no | |
| `expires_at` | ts | no | |

Indexes: `sessions_user_idx` (`user_id`), `sessions_expires_idx` (`expires_at`).

### password_resets

Platform table. **[P-22]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `password_resets_pkey` |
| `user_id` | id | no | FK → `users(id)` ON DELETE CASCADE `password_resets_user_fkey` |
| `token_hash` | text | no | UNIQUE `password_resets_token_hash_key` |
| `created_by` | id | yes | FK → `users(id)` ON DELETE SET NULL; null when created by the CLI |
| `created_at` | ts | no | |
| `expires_at` | ts | no | |
| `used_at` | ts | yes | |

Index `password_resets_user_idx` (`user_id`).

### auth_throttle

Platform table. **[P-18]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `key` | text | no | PK `auth_throttle_pkey`; `id:<identifier>` or `ip:<address>` |
| `failures` | int | no | ≥ 0 |
| `window_started_at` | ts | no | |
| `locked_until` | ts | yes | |

### setup_tokens

Platform table. **[P-24]** At most one row.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `token_hash` | text | no | PK `setup_tokens_pkey` |
| `created_at` | ts | no | |
| `expires_at` | ts | no | |

## Later steps

Tables for songs, readings, templates, services, liturgies, comments, edits, published versions and imports are added in the steps that build them, following [SPEC.md §7](../SPEC.md#7-data-model-sketch).

## References

| Topic | Location |
|---|---|
| Conceptual data model | [SPEC.md §7](../SPEC.md#7-data-model-sketch) |
| Type mapping, migrations, error mapping | [02-persistence.md](../impl/02-persistence.md) |
| Identity and auth rules using these tables | [03-identity-auth.md](../impl/03-identity-auth.md) |
| Tenancy rules | [SPEC.md §8.1](../SPEC.md#81-tenancy) |
