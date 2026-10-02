# Database Schema (Reference)

> **Document type: Reference.** Tables that exist after each build step. The step-1 tables are: translations, churches, users, memberships, roles, role_scopes, membership_roles, invites, invite_roles, sessions, password_resets, auth_throttle, setup_tokens. Conceptual model: [SPEC.md §7](../SPEC.md#7-data-model-sketch). Type mapping, migrations and error mapping: [02-persistence.md](../impl/02-persistence.md).
> Status: **Approved** 2026-10-02 for step 1, including the round-2 review fixes. Proposal markers refer to the [index](../impl/README.md#4-proposed-decisions).

## Conventions

- Types below are logical; physical types per database follow [02 §4](../impl/02-persistence.md#4-type-mapping): `id` = ULID text (26 chars), `ts` = UTC timestamp at microsecond precision, `json` = JSON value, `bool`, `text`, `int`.
- SQLite tables are `STRICT`. Every constraint and index name below is identical in both migrations.
- **Enforcement marker:** a rule written as SQL is enforced by the database; the SQL is given per dialect where it differs. A rule marked **(app)** is enforced only by the application, through the `domain` constructors used by every write path, and covered by a repository test that attempts a non-conforming write. Case rules are app-only because Unicode case folding differs between Go, SQLite and PostgreSQL.
- **Platform tables** have no `church_id`. **Church-owned tables** have `church_id`, composite foreign keys including `church_id` ([SPEC.md §8.1](../SPEC.md#81-tenancy) rule 1), and a **church-leading index**: an index, primary key or unique constraint whose first column is `church_id`. A table without one must list the exception and its reason here (none in step 1; `song_fts` in step 2).
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

## Step 2 tables

Added by migration `00002_library.sql` (both dialects, same version). Church-owned, with the church-leading indexes the conventions require.

### song_groups

Church-owned. Links the language versions of one hymn **[P-46]**.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `song_groups_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `song_groups_church_fkey` |
| `created_at` | ts | no | |

- UNIQUE `song_groups_church_id_key` (`church_id`, `id`) — target for composite foreign keys; church-leading.
- A group with fewer than two songs is deleted by the application in the same transaction (the foreign key from `songs` is `ON DELETE RESTRICT`, so the application first clears `song_group_id`).

### songs

Church-owned. **[P-46] [P-53]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `songs_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `songs_church_fkey` |
| `song_group_id` | id | yes | FK (`church_id`, `song_group_id`) → `song_groups(church_id, id)` ON DELETE RESTRICT `songs_group_fkey` |
| `language` | text | no | `CHECK (language IN ('id','en','zh-Hans','zh-Hant'))` `songs_language_check` |
| `title` | text | no | 1–200 chars (app) |
| `title_key` | text | no | `Fold(title)` (app); used for ordering and duplicate detection |
| `alt_titles` | json | no | Array of 0–10 strings (app); `[]` when none |
| `hymnal_source` | text | no | 0–40 chars (app); empty string when none |
| `hymnal_number` | text | no | 0–10 chars (app); empty string when none |
| `hymnal_key` | text | yes | `<SOURCE KEY>:<number lower-case>` (app); null iff `hymnal_number = ''` |
| `lyricist`, `composer`, `translator` | text | no | 0–200 chars (app); empty string when none |
| `default_key` | text | no | Empty or `^[A-G][#b]?m?$` (app) |
| `copyright_holder` | text | no | 0–200 chars (app) |
| `copyright_line` | text | no | 0–300 chars (app) |
| `ccli_song_number` | text | no | Empty or 1–12 digits (app) |
| `licence_status` | text | no | `CHECK (licence_status IN ('unknown','public_domain','church_licence','permission_obtained'))` `songs_licence_status_check` |
| `licence_notes` | text | no | 0–2000 chars (app) |
| `default_arrangement` | json | no | Array of 0–100 section IDs of this song (app); `[]` when none |
| `version` | int | no | `CHECK (version >= 1)` `songs_version_check` |
| `created_at` | ts | no | |
| `updated_at` | ts | no | |

- `CHECK ((hymnal_source = '') = (hymnal_number = '') AND (hymnal_key IS NULL) = (hymnal_number = ''))` `songs_hymnal_check`.
- UNIQUE `songs_church_id_key` (`church_id`, `id`) — target for composite foreign keys; church-leading.
- UNIQUE partial index `songs_group_language_key` (`church_id`, `song_group_id`, `language`) `WHERE song_group_id IS NOT NULL` — at most one song per language in a group.
- Index `songs_church_title_idx` (`church_id`, `title_key`, `id`) — list order.
- Partial index `songs_church_hymnal_idx` (`church_id`, `hymnal_key`) `WHERE hymnal_key IS NOT NULL` — duplicate detection and hymnal lookups. Not unique.
- Index `songs_church_licence_idx` (`church_id`, `licence_status`).

### song_sections

Church-owned. Order is `position`; IDs are stable **[P-46]**.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `song_sections_pkey` |
| `church_id` | id | no | |
| `song_id` | id | no | FK (`church_id`, `song_id`) → `songs(church_id, id)` ON DELETE CASCADE `song_sections_song_fkey` |
| `position` | int | no | `CHECK (position >= 0)` `song_sections_position_check`; dense 0..n-1 per song (app); not unique, so a reorder can be written row by row |
| `kind` | text | no | `CHECK (kind IN ('verse','pre_chorus','chorus','bridge','tag','intro','ending','other'))` `song_sections_kind_check` |
| `number` | int | yes | `CHECK ((kind = 'verse' AND number BETWEEN 1 AND 99) OR (kind <> 'verse' AND number IS NULL))` `song_sections_number_check` |
| `label` | text | yes | 1–60 chars (app); null = derived from kind and number |
| `text` | text | no | 1–5000 chars (app), normalised |

- UNIQUE partial index `song_sections_verse_key` (`song_id`, `number`) `WHERE kind = 'verse'` — no two verses with one number.
- Index `song_sections_church_song_idx` (`church_id`, `song_id`, `position`) — church-leading.

### song_search

Church-owned. Search index, written by the songs repository **in the same transaction** as every song change; rebuildable with `liturgist search reindex` **[P-48]**.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `church_id` | id | no | |
| `song_id` | id | no | |
| `language` | text | no | Copy of `songs.language` |
| `title_fold` | text | no | Folded title and alternative titles |
| `hymnal_fold` | text | no | Folded hymnal source and number |
| `lyrics_fold` | text | no | Folded section texts, in order |
| `fts` | tsvector | no | **PostgreSQL only**; `to_tsvector('simple', …)` of the three `_fold` columns, maintained by the application (no generated column, so both dialects share the write path) |

- PK `song_search_pkey` (`church_id`, `song_id`) — church-leading.
- FK `song_search_song_fkey` (`church_id`, `song_id`) → `songs(church_id, id)` ON DELETE CASCADE.
- PostgreSQL: GIN index `song_search_fts_idx` on `fts`.
- SQLite: the FTS5 virtual table `song_fts(title, hymnal, lyrics, church_id UNINDEXED, song_id UNINDEXED)`, tokenizer `unicode61`, `prefix='2 3'`. A virtual table has no keys or cascades, so the repository deletes its rows explicitly whenever it deletes a `song_search` row. **Exception to the church-leading-index rule**: `song_fts` has none, because every query filters on `church_id` through the join with `song_search`; the exception is listed here as the conventions require.

### readings

Church-owned. **[P-50]**

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `readings_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `readings_church_fkey` |
| `reference` | text | no | Standard form, e.g. `JHN 3:16-21` (app, `domain.ParseReference`) |
| `reference_display` | text | no | As typed, 1–100 chars (app) |
| `translation_id` | id | no | FK → `translations(id)` `readings_translation_fkey` |
| `text` | text | no | 1–20000 chars (app), normalised |
| `attribution` | text | no | 0–300 chars (app) |
| `source_provider` | text | no | `manual` or a provider ID, 1–40 chars (app) |
| `search_fold` | text | no | Folded `reference_display`, Indonesian book name and text (app); substring search |
| `version` | int | no | `CHECK (version >= 1)` `readings_version_check` |
| `created_at` | ts | no | |
| `updated_at` | ts | no | |

- UNIQUE `readings_church_ref_key` (`church_id`, `reference`, `translation_id`) — church-leading.

### import_batches

Church-owned. Working data, deleted after 7 days **[P-51]**.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `import_batches_pkey` |
| `church_id` | id | no | FK → `churches(id)` ON DELETE CASCADE `import_batches_church_fkey` |
| `source_format` | text | no | `CHECK (source_format IN ('paste','openlyrics','chordpro','easyworship','pptx'))` `import_batches_format_check` |
| `status` | text | no | `CHECK (status IN ('open','closed'))` `import_batches_status_check` |
| `created_by` | id | no | FK → `users(id)` `import_batches_user_fkey` |
| `created_at` | ts | no | |
| `updated_at` | ts | no | Set by every change; the cleanup job deletes batches whose `updated_at` is older than 7 days |

- UNIQUE `import_batches_church_id_key` (`church_id`, `id`).
- Index `import_batches_church_status_idx` (`church_id`, `status`, `updated_at`) — church-leading.

### import_candidates

Church-owned.

| Column | Type | Null | Constraint |
|---|---|---|---|
| `id` | id | no | PK `import_candidates_pkey` |
| `church_id` | id | no | |
| `batch_id` | id | no | FK (`church_id`, `batch_id`) → `import_batches(church_id, id)` ON DELETE CASCADE `import_candidates_batch_fkey` |
| `position` | int | no | `CHECK (position >= 0)`; order in the file(s) |
| `kind` | text | no | `CHECK (kind IN ('song','reading'))` `import_candidates_kind_check`; step 2 creates only `song` |
| `draft` | json | no | A `SongDraft` (app validates the shape on every write) |
| `duplicate_of_id` | id | yes | The suspected duplicate when the batch was created; **not** a foreign key (the song may be deleted) |
| `decision` | text | no | `CHECK (decision IN ('pending','accept','merge','skip'))` `import_candidates_decision_check` |
| `merge_into` | id | yes | Song to merge into; not a foreign key; `CHECK ((decision = 'merge') = (merge_into IS NOT NULL))` `import_candidates_merge_check` |
| `warnings` | json | no | Array of warning codes |
| `applied_song_id` | id | yes | Set by Apply; not a foreign key |
| `error_code` | text | yes | Set by Apply when a candidate failed |

- Index `import_candidates_church_batch_idx` (`church_id`, `batch_id`, `position`) — church-leading.

### Unique constraints added to the SQLite name mapping ([02 §8](../impl/02-persistence.md#8-error-mapping))

`songs_group_language_key`, `song_sections_verse_key`, `readings_church_ref_key`.

## Later steps

Tables for templates, services, liturgies, comments, edits and published versions are added in the steps that build them, following [SPEC.md §7](../SPEC.md#7-data-model-sketch) and the conventions above.

## References

| Topic | Location |
|---|---|
| Conceptual data model | [SPEC.md §7](../SPEC.md#7-data-model-sketch) |
| Type mapping, migrations, error mapping, atomic operations | [02-persistence.md](../impl/02-persistence.md) |
| Identity and auth rules using these tables | [03-identity-auth.md](../impl/03-identity-auth.md) |
| Tenancy rules | [SPEC.md §8.1](../SPEC.md#81-tenancy) |
| Review findings this version answers | [schema-adversarial-review.md](schema-adversarial-review.md), [review log](../impl/README.md#6-review-log) |
