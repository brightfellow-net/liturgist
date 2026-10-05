# 10 — Liturgies: Items, Songs, Readings, Assignments (Implementation)

> **Document type: Implementation.** Step 3 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slice 3B.
> Status: decisions **[P-xx] Approved** 2026-10-03 (after the adversarial review; conditions in the [index](README.md#4b-proposed-decisions-step-3)). Code may be written from this document. Items marked **[P-xx]** are decisions listed in the [index](README.md#4b-proposed-decisions-step-3). Nothing here is Approved until the owner says so.

## 1. Scope

Creating liturgies (one at a time, or a whole week), their items, the songs and sequences in song items, the reading of a reading item, assignments of people to duties, concurrent-edit rules, the edit history, and the real `SongUsage` and `ReadingUsage`. Duties, singing parts, templates and services are in [09](09-planning.md); the pages in [11](11-liturgy-editor.md).

| Left out | Why | Where it lands |
|---|---|---|
| Review states, submit, approve, comments | Step 4. A liturgy is created in `draft`; the schema already allows every state and the edit rules below already check the state | Step 4 |
| Publishing, archiving, `PublishedVersion`, "my assignments", PDF | Steps 4–5 | Steps 4–5 |
| "Archive last week's liturgies and create these" button of "Prepare next week" | Needs published liturgies; only matters with the SaaS free plan | Step 5 |
| Undo and redo | Slice 3D builds them on the records this slice writes | [11 §7](11-liturgy-editor.md#7-undo-and-redo-slice-3d) |
| Non-lyric sequence entries (instrumental, spoken) | SPEC §5.2 | After the MVP; `sequence_entries.kind` allows it |

## 2. Data model

Domain types in `domain/liturgy.go`; tables in [reference/schema.md](../reference/schema.md#step-3-tables).

### 2.1 Liturgy

| Field | Rule |
|---|---|
| `date` | Calendar date `YYYY-MM-DD` (no time zone) |
| `time` | `HH:MM` in the church's time zone, or empty for a one-off service with no time. Required (non-empty) when `service_id` is set |
| `service_id` | Optional service of this church; null for one-off services and after the service is deleted |
| `service_name` | 1–100 characters; a **copy** of the service's name at creation, editable |
| `language` | `id`, `en`, `zh-Hans`, `zh-Hant`; **fixed at creation** (§3) |
| `template_id` | The template it was made from (informational; null if none or deleted) |
| `state` | `draft`, `in_review`, `needs_revision`, `approved`, `published`. Step 3 creates only `draft` |
| `version` | Starts at 1; +1 for every **structural** change (§5) |
| `archived_at`, `archived_by` | Null in step 3 (set by archiving, step 5) |
| `created_by`, `created_at`, `updated_at` | |

- **One liturgy per service slot [P-58]:** a *slot* is (`service_id`, `date`, `time`), and a church has at most one liturgy per slot (partial unique index; 409 `liturgy_exists` with `liturgy_id`). A `time` that is not one of the service's times (a special start) is a different slot, so a service may have two liturgies on one date at different times; two different services may share a date and time; one-off liturgies (no `service_id`) are not limited. The unique index is the same on both dialects. Two services sharing a slot was decided by the owner on 2026-10-03 (Q-3.7 in the [index](README.md#4c-questions-for-the-owner-step-3)).
- **Editable** exactly in states `draft` and `needs_revision` (SPEC §5.5). A write to a liturgy in any other state → 409 `liturgy_locked`, nothing written.
- **Visible** to members holding any of `liturgy.edit`, `liturgy.comment`, `liturgy.approve`, `liturgy.manage`; for anyone else the liturgy is 404 **in every state** [P-64, amended by P-79]. Members without such a scope read a published liturgy only through its published copy ([13 §5](13-publishing.md#5-reading-the-published-copy-p-79-p-80)), never through the routes of this document or its edit history.
- **Deletable** in the four unpublished states by `liturgy.manage`, **but never once any `PublishedVersion` exists** (409 `liturgy_not_deletable`, also for a reopened liturgy: it can only be archived after it is published again) [P-75, amended by 13 §2]. The delete itself is conditional on the state and on the absence of versions. Deleting removes items, songs, entries, assignments and history in the same transaction (`ON DELETE CASCADE`).

### 2.2 Item

| Field | Rule |
|---|---|
| `title` | 1–200 characters |
| `item_type` | `song`, `reading`, `prayer`, `sermon`, `free_text`, `other`; **immutable** after creation [P-61] |
| `duty_id` | Optional duty of this church: who does this item (shown with the assigned people). Copied from the template item |
| `text` | 0–20,000 characters (`\n` line endings); only for `prayer`, `sermon`, `free_text`, `other` (422 otherwise) |
| `reading_id` | `reading` items only: a reading of this church; null = "not chosen yet" |
| `reading_label` | Snapshot of the reading's display text (`Yohanes 3:16-21 (TB)`), set with `reading_id` and kept when the reading is later deleted |
| `position` | Dense 0..n-1 per liturgy; changed only by add, remove and reorder |
| `version` | Starts at 1; +1 for every change to this item's own fields, songs or sequences |
| Limit | 60 items per liturgy |

A `song` item holds 0–10 **item songs** (§2.3); a `reading` item holds one reading; the other kinds hold `text`. Medley = several item songs, sung in order.

### 2.3 Item song and sequence

| Field of an item song | Rule |
|---|---|
| `song_id` | A song of this church; set to null if the song is deleted later (§6) |
| `song_title` | Snapshot of the title, kept when the song is deleted. Also in the GET as `song: null` with the snapshot when the song is gone, so the row still shows a name |
| `key` | Empty or `^[A-G][#b]?m?$`; defaults to the song's `default_key` when added |
| `note` | 0–200 characters |
| `position` | Dense 0..n-1 within the item |
| `entries` | The **sequence**: 0–100 entries (the same limit as an arrangement) |

| Field of a sequence entry | Rule |
|---|---|
| `kind` | `section` (the only kind in the MVP) |
| `song_section_id` | A section **of that item song's song**; set to null if the section is deleted later (§6) |
| `singing_part_id` | Optional singing part of this church: who sings it |
| `key_change` | Empty or a key as above: the key the music changes to at this entry |
| `note` | 0–100 characters (e.g. "2x") |
| `section_label` | Snapshot of the section's display text ("Verse 1", "Chorus"), set with `song_section_id` and **kept** when the section is deleted |

**Filling the sequence [P-62]:** when a song is added with `POST …/songs`, the server creates the entries from the song's default arrangement; if the song has none, from **all its sections in their order** (this is how [06 §2.4](06-song-library.md#24-editing-sections-and-concurrency-p-46) defines an empty arrangement; SPEC §5.2's "all verses in order" is read the same way, so choruses are not dropped). A song with no sections gets an empty sequence. `key` defaults from the song. Everything after that is edited freely with `PUT …/songs`.

### 2.4 Assignment [P-63]

A person (a member, or a free-text name) assigned to a duty in one liturgy.

| Field | Rule |
|---|---|
| `duty_id` | Duty of this church |
| `user_id` | Exactly one of `user_id` and `name`. The user must hold a membership of this church **when assigned** |
| `name` | 1–100 characters (people without an account; not counted as team members, [SPEC §11](../SPEC.md#11-decisions-log)) |

Unique per liturgy: (`duty_id`, `user_id`) and (`duty_id`, `Fold(name)`) → 409 `assignment_exists`. Several people may share a duty (musicians). A person may hold several duties. Limit 200 assignments per liturgy. If a member is later removed from the church, their assignments stay and are shown as "(no longer a member)": the history of a liturgy is not rewritten. `former_member` is **computed** on every read (the user holds no membership of this church now), never stored, so it is always consistent with the membership read in the same transaction. Creating an assignment takes `LockChurch` (§6), and so does removing a membership ([02 §3](02-persistence.md#3-connections-and-transactions)), so an assignment is either made while the person is a member or refused; it never ends up for a person who was removed first.

### 2.5 Problems

`GET /liturgies/{id}` returns `problems`, a computed list (never stored) the editor shows and step 4 will check before a submit: `{ code, item_id, item_song_id?, entry_id? }` with codes `song_missing` (a song item with no songs), `reading_missing` (reading item without a reading), `song_removed` (an item song whose song was deleted), `reading_removed`, `section_removed` (an entry whose section was deleted). Step 3 only reports them; nothing is refused.

## 3. Creating a liturgy [P-58]

`POST /liturgies` with `date`, and either:

- `service_id` and `time` (any `HH:MM`, not only one of the service's times, so a special start time is possible), or
- `service_name` for a one-off.

Rules, in one `Tx.Write` under `LockChurch`:

1. `liturgy.edit`; the entitlement limits of §8 are checked first.
2. **Template:** `template_id` if given, else the service's `default_template_id`, else none (an empty liturgy). Items are copied in template order: title, type, `default_text` → `text`, `default_duty_id` → `duty_id`.
3. **Language:** the request's `language` if given, else the service's, else the template's, else the church's default content language. Fixed afterwards. The chosen template must have the same language as the liturgy, or 422 `validation_failed` (`field: "template_id"`, `reason: "language_mismatch"`): a liturgy never holds items worded in another language than itself. So a request may override the service's language only together with a template of that language (or with `template_id: ""` for an empty liturgy). This refines SPEC §5.2's "the language comes from the service/template"; the SPEC text is read as the default, not as a ban on a deliberate choice.
4. **Name:** `service_name` is the service's name, or the request's for a one-off; a request value overrides for a service too.
5. Slot uniqueness: 409 `liturgy_exists`.
6. History: one `liturgy.create` edit ([§7](#7-history)).

### 3.1 Prepare next week [P-59]

`GET /liturgies/prepare?week=YYYY-MM-DD` (with `POST`, and `GET /liturgies/assignable`, it requires `liturgy.edit`; 403 otherwise) lists **occurrences**: for the week containing `week` (Monday to Sunday, ISO 8601, computed on the calendar date; the church's `time_zone` is an IANA name that `time.LoadLocation` accepts, enforced when it is saved, [03 §10](03-identity-auth.md#10-first-time-setup); it is a wall-clock zone, so a changed zone changes only which calendar week "next week" means, never stored dates and times) and every service, each `times` entry gives one occurrence `{ service_id, service_name, language, date, time, template_id, template_name, liturgy_id? }`; `liturgy_id` is set when a liturgy for that slot exists, and the web app leaves those unticked and disabled. Without `week`, the week after the current one in the church's time zone. Sorted by date, then time, then service name. The response also has `limits` ([§8](#8-entitlements-and-limits)): for each of the two limits `{ unlimited, max, used }`.

`POST /liturgies/prepare` with `occurrences: [{ service_id, date, time }]` (1–50) and, from step 5, an optional `archive_ids` that archives earlier published liturgies in the same transaction ([13 §4](13-publishing.md#4-reopening-archiving-unarchiving-p-77-p-78)) creates one liturgy per entry as in §3 with the service's template and language **as they are when the request is processed**; the preview is not a promise, and the response lists what was created (name, language, template) so a changed service is visible. A service whose template has another language than the service → that occurrence fails with 422 (`reason: "language_mismatch"`), which fails the batch. **All or nothing** in one transaction: an entry that is not a real occurrence of that service (its weekday and `time` are not in the service's `times`) → 422; an existing slot → 409 `liturgy_exists` naming the first; the limits are checked once for the whole batch (`used + n > max` → 403 `limit_reached`). The response lists the created liturgies.

## 4. Editing items, songs and assignments

Every route below needs `liturgy.edit`, a visible liturgy, and an editable state (409 `liturgy_locked`). Request versions are described in §5.

| Method & path | Request | Effect |
|---|---|---|
| `PATCH /liturgies/{id}` | `{ version, date?, time?, service_name? }` | Structural. Changing `date`/`time` of a service liturgy can collide → 409 `liturgy_exists` |
| `POST /liturgies/{id}/items` | `{ liturgy_version, title, item_type, duty_id?, text?, position? }` | Inserts at `position` (0..n, default n); item version 1; 422 beyond 60 items |
| `PATCH /liturgies/{id}/items/{iid}` | `{ version, title?, duty_id?, text?, reading_id? }` | `duty_id: ""` clears; `reading_id: ""` clears; setting `reading_id` fills `reading_label`; an unknown/foreign reading → 422 |
| `DELETE /liturgies/{id}/items/{iid}?liturgy_version=n` | — | Removes the item with its songs and entries. **200** `{ liturgy_version, item_ids }` (the new version and the remaining order), not 204: the client needs the new version to continue |
| `PUT /liturgies/{id}/items/order` | `{ liturgy_version, item_ids }` | Exactly the current IDs once each, else 409 `version_conflict` (`scope: "liturgy"`) |
| `POST /liturgies/{id}/items/{iid}/songs` | `{ version, song_id, position? }` | Adds a song with its sequence filled (§2.3); 422 if the item is not a `song` item or already has 10 songs |
| `PUT /liturgies/{id}/items/{iid}/songs` | `{ version, songs: [{ song_id, key, note, entries: [{ section_id, singing_part_id?, key_change?, note? }] }] }` | Destructive replacement of the complete list: **every item-song and entry ID is new** and clients must treat all earlier child IDs as invalid; each `section_id` must belong to its `song_id` (else 422); the response is the item |
| `GET /liturgies/assignable` | — | `{ items: [{ user_id, name }] }`: **every** member of the church (owner decision Q-3.8, 2026-10-03) with the display name and nothing else (no e-mail, role or contact data), for `liturgy.edit` (so an editor without `members.view` can assign). It is the minimum an assignment picker needs; the full member list stays behind `members.view` |
| `POST /liturgies/{id}/assignments` | `{ duty_id, user_id? , name? }` | 201 assignment; not versioned |
| `DELETE /liturgies/{id}/assignments/{aid}` | — | 204 |

Reads (members who can see the liturgy; `GET /liturgies` returns only the liturgies the caller may see, so for a member without a `liturgy.*` scope it is an empty list in step 3, not an error): `GET /liturgies?state=&from=&to=&order=&limit=&offset=` → `{ items: [{ id, date, time, service_name, language, state, item_count, version, actions }], total }`; `from`/`to` are inclusive dates, `order` is `date_asc` or `date_desc` (default), ties broken by time, then ID (`limit` default 50, at most 100); `GET /liturgies/{id}` → the liturgy with `items` (each with `songs` and `entries`, and `reading` summary), `assignments`, `problems`, `version`, `actions: { edit, delete }` (`edit` = holds `liturgy.edit` **and** the state is editable; `delete` = holds `liturgy.manage` **and** the state is unpublished; both are about the liturgy's own state, which is already loaded, and never about a lookup such as a usage count — see [04 §5](04-tenancy-extensions.md#5-authorization)). Each item song carries a summary of its song — `title`, `language`, `hymnal_source`, `hymnal_number`, `default_key` and its `sections` as `{ id, kind, number, label }` without text — and each reading item a summary of its reading, so the page needs no second request; **lyrics are not included** in this response (a reading item includes its text, a song item does not) — the editor shows a section's text from `GET /songs/{id}` only on demand.

Route order: the fixed paths `prepare` and `assignable` are registered before `{id}`; an ID (a ULID) can never equal them.

Authorization: no session → 401; not a member, or a liturgy the member may not see → 404 (P-26); a member with visibility but without the write scope → 403.

Logging: `liturgy_created`, `liturgies_prepared` (count), `liturgy_updated`, `liturgy_deleted`, `item_added`, `item_updated`, `item_removed`, `items_reordered`, `item_songs_set`, `assignment_added`, `assignment_removed`, with actor and IDs. Titles and texts are never logged.

## 5. Versions and conflicts [P-60]

Two counters: the liturgy's `version` guards **structure** (which items exist and in what order, and the liturgy's own fields); an item's `version` guards **that item's content**. Editing different items never conflicts.

| Operation | Must state | Checked against | Bumps |
|---|---|---|---|
| `PATCH` liturgy, add item, remove item, reorder | `version` / `liturgy_version` | the liturgy's version | the liturgy's version |
| `PATCH` item, add song, `PUT` songs | item `version` | the item's version | the item's version |
| Assignments | nothing | uniqueness only | nothing |

- **Order inside a write transaction:** (1) check the state; (2) the conditional update that bumps the version of the *parent* (the item for item writes, the liturgy for structure writes) — this claims the row, and a second writer based on the same version fails here, before touching any child; (3) child rows (songs, entries, positions); (4) `UPDATE liturgies SET edit_seq = edit_seq + 1 … RETURNING edit_seq` (this takes the liturgy row's lock until commit, so the history order is the commit order); (5) the `liturgy_edits` row with that `seq`. Assignments, which have no version, do steps (1), (3), (4), (5). Undo and redo do step (4) **first**, so their conflict test sees every committed edit. Any failure rolls the whole transaction back. This applies to every route in §4, including `PUT …/songs` and reorder.
- All checks are conditional updates (`… WHERE id = $1 AND version = $2`, [02 §2.1](02-persistence.md#21-atomic-operations)); zero rows changed and the row exists → 409 `version_conflict` with `scope` (`liturgy` or `item`) and `item_id`; nothing is written. The row gone → 404 `not_found`.
- A response to a write carries the new version(s); the client keeps them.
- Item content edits do **not** touch the liturgy's `version`, so a reorder is refused only when items were added, removed or reordered meanwhile.
- A write to a liturgy whose state is not editable is refused before the version check (`liturgy_locked`).

## 6. Usage ports and deleted songs, readings, sections

One `UsageRepo` (`ChurchStore.Usage()`, bound to the open transaction so its answers are read under `LockChurch`) replaces the step 2 ports `SongUsage` and `ReadingUsage` ([06 §6](06-song-library.md#6-ports), [07 §6](07-readings.md#6-ports)), the planning usage port of [09](09-planning.md) and `app.NeverUsed`. Duties and singing parts count as in use when **any** liturgy refers to them, whatever its state; songs, sections and readings follow the rules below:

- A song or reading is **in use** when an item song / item refers to it in a liturgy whose state is `draft`, `in_review`, `needs_revision` or `approved`.
- A section is in use when an entry of such a liturgy refers to it.
- **Races:** every write that adds a reference to a song, section, reading, duty or singing part (adding an item or song, `PUT …/songs`, setting `reading_id` or `duty_id`, assignments) and every delete of one of those takes `LockChurch` ([02 §3](02-persistence.md#3-connections-and-transactions)), so a delete cannot pass the usage check while a liturgy is adding the reference. **Removing sections is such a delete:** `PATCH /songs/{id}` (a full-list replacement, [06 §2.4](06-song-library.md#24-editing-sections-and-concurrency-p-46)) takes `LockChurch` whenever the new list drops a section, runs the usage check and deletes the sections under that lock in one transaction, and takes the lock *before* its song-version update (the same order as every other writer: church lock first, then row versions). `DELETE /readings/{id}` takes the lock too. A foreign-key refusal that still happens is mapped to the matching `*_in_use` 409, never a 500.
- **PostgreSQL lock order:** an item edit locks the item row and then the liturgy row (the `edit_seq` update), while a structure write locks the liturgy row and then the item rows, so the two can deadlock on PostgreSQL. `Tx.Write` retries a deadlock victim ([02 §3](02-persistence.md#3-connections-and-transactions)), so a victim is re-run (at most 3 attempts, then 503 `unavailable`); `TestMixedWritesEndInSuccessOrConflict` stresses it and saw no error. Do not take a third lock order.
- Deleting a song, reading or section that only **published** liturgies use is allowed (a published liturgy has its own copy in `PublishedVersion`, step 5). The editable rows that still refer to it are set to null by the delete itself (one repository method nulls the references and deletes, in one transaction — the foreign keys stay `RESTRICT`, [schema](../reference/schema.md#step-3-tables)) and show as problems (`song_removed`, `reading_removed`, `section_removed`) if that liturgy is reopened. Snapshots keep the display text (§2.3): `song_title`, `reading_label` and, for entries, `section_label`. Whether a reopened liturgy full of removed references is acceptable, and what `PublishedVersion` keeps, is decided with the review states in step 4 (it needs the publish rules); this document only guarantees that nothing in step 3 breaks (no 500, the GET shows the snapshots, the editor can replace each removed reference).

## 7. History

Every successful change — every route of §4 and every undo or redo of slice 3D; a liturgy created from a template writes one `liturgy.create` row whose image holds the copied items — writes exactly one `liturgy_edits` row **in the same transaction**: `id`, `liturgy_id`, `seq`, `user_id`, `command`, `target_edit_id?` (undo and redo rows only), `item_id?`, `before`, `after` (JSON images), `liturgy_version_after`, `item_version_after?` (informational), `status` (`done`; `undone` and `dropped` are set by slice 3D, which also fills `undo_seq?` and sets `skipped`, 11 §7.2), `created_at`. `seq` is the number the write took from `liturgies.edit_seq` by `UPDATE liturgies SET edit_seq = edit_seq + 1 … RETURNING`, the last write before the row is inserted ([§5](#5-versions-and-conflicts-p-60)); writers of one liturgy therefore commit in `seq` order, and slice 3D uses `seq`, not times or versions, to say what happened after what. The history is visible to everyone who can see the liturgy: `GET /liturgies/{id}/edits?limit=` (newest first, at most 100). Slice 3D adds undo and redo, which use the images; this slice only writes them, so its rules are testable without undo.

| `command` | `before` | `after` |
|---|---|---|
| `liturgy.create` | null | the new liturgy's fields and items |
| `liturgy.update` | `date`, `time`, `service_name` before | after |
| `item.add` | null | the complete item (position, fields, songs, entries) |
| `item.remove` | the complete item | null |
| `item.update` | the changed fields before | after |
| `item.songs` | the complete songs list with entries | after |
| `items.reorder` | the ID list | the new ID list |
| `assignment.add` / `assignment.remove` | null / the row | the row / null |
| `undo` / `redo` (slice 3D) | the image of the state before the undo / redo is applied | the image after it; `target_edit_id` names the edit acted on |

An image holds IDs and field values, never lyrics. **There is no size cap and nothing is truncated or refused**: the limits of §2 bound every image. The largest legal ones are an `item.songs` / `item.remove` image (10 songs × 100 entries × about 300 bytes of JSON ≈ 300 KB) and a `liturgy.create` image (60 template items of 5,000 characters ≈ 300 KB); an `item.update` image holds at most two copies of a 20,000-character text. TC-L-010 builds the worst legal case for each command and checks that it is stored and read back whole. The columns are `text` holding JSON, without a length limit (SQLite has no JSON type, and the same column type keeps both dialects alike). History is deleted with the liturgy and is not trimmed in step 3 (a church that edits for years may want a retention rule; that is a later decision, not a silent loss now).

**Undo and redo rows** are history like any other: they are listed, they carry their own `seq`, and the `command` `undo` / `redo` makes them **never** an undo or redo target and never part of the 50-edit window; for the conflict test of [11 §7.2](11-liturgy-editor.md#72-undo-and-redo-p-65) they count as the author's touch of the item or structure they changed. Restoring a deleted item re-creates it with its old ID and `version` = the version it had in the image of the row that removed it, plus 1 ([11 §7.2](11-liturgy-editor.md#72-undo-and-redo-p-65); the number never goes back to a value it had), the liturgy's `version` +1 for the structural change, and the old `position` clamped to the current count.

## 8. Entitlements and limits

`Entitlements.Limit(church, name)` ([04 §8](04-tenancy-extensions.md#8-entitlements)) is consulted when liturgies are created (and, in step 5, when a published liturgy is reopened or unarchived, [13 §4](13-publishing.md#4-reopening-archiving-unarchiving-p-77-p-78), with a message per limit):

| Limit | Counts | Check |
|---|---|---|
| `max_active_liturgies` | liturgies with `archived_at` null, any state | `used + new > max` → 403 `limit_reached` (`limit`, `used`, `max`) |
| `max_unpublished_liturgies` | liturgies in `draft`, `in_review`, `needs_revision`, `approved`, not archived | same |

The community stub answers "unlimited". Counting and creating happen in one transaction under `LockChurch`, so two requests cannot both take the last slot. Editing, deleting and reading are never limited.

## 9. Ports

| Port | Methods | Implemented by | Used by |
|---|---|---|---|
| `ChurchStore.Liturgies()` | create, `ByID`, `List`, `Count(archived, states)`, conditional update on `version`, delete | `adapters/sqlstore` | Liturgy use cases |
| `ChurchStore.LiturgyItems()` | insert, `ByID`, conditional update on `version`, delete, `Reorder`, `ReplaceSongs` | same | Item use cases |
| `ChurchStore.Assignments()` | add, remove, `ByLiturgy` | same | Assignment use cases |
| `ChurchStore.Edits()` | `Append`, `List` | same | Every write |
| `SongUsage`, `ReadingUsage` | as in 06 §6 / 07 §6 | `adapters/sqlstore` | Song and reading delete |
| `EventBus` | Not used by step 3 while live updates are on the icebox ([11 §7.1](11-liturgy-editor.md#71-live-updates-and-presence-icebox-p-66)) | in-memory (community) | — |

The use cases do not publish events.

## 10. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Update an item without stating its version | Conditional update; 409 `version_conflict` with `scope` | Two editors must not silently overwrite each other (SPEC §5.2) |
| Bump the liturgy version on every item edit | Item edits bump only the item | Editing different items would conflict all the time |
| Check "editable" only in the web app | Check the state in every write use case | Locked liturgies must stay locked for every client (SPEC §5.5) |
| Count liturgies and then insert in two transactions | One transaction under `LockChurch` | Two requests would both take the last free slot |
| Copy lyrics into the liturgy | Refer to the song and its sections; lyrics are read from the library | SPEC §3: enter once; publishing (step 5) takes the copy |
| Let deleting a song fail because an old published liturgy used it | Allow it; set references to null and keep the snapshots | History must not block library cleaning; the published copy is separate |
| Allow changing an item's type | Delete the item and add another | Changing type would silently drop songs or text, and undo would need to restore them |
| Store the sequence as the arrangement of the song | A separate entry list per item song | The liturgy edits its sequence freely without touching the library |
| Show "Do = G" by storing it | Store the letter key; the web app formats it | One value for the sort, print and WhatsApp text; the display is a church setting |
| Log item text, song titles with lyrics or notes | IDs only | Privacy and copyright |

## 11. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-L-001 | Liturgy validation | Date `2026-10-04`, `2026-02-30`, `4-10-2026`; time `07:00`, `7:00`, `24:00`, empty with and without a service | Rules of §2.1 | One-off with empty time valid; service with empty time invalid |
| TC-L-002 | Create from template | Template of seven items, a service with another language | Items copied in order with text and duty; language of the service; edits to the template afterwards do not change the liturgy | No template → empty liturgy; request language overrides |
| TC-L-003 | Week and occurrences | Services "Sunday 07:00", "Sunday 09:00, Wednesday 19:00"; `week` on a Wednesday, on a Sunday, on a Monday | The same Monday–Sunday week for all three; 3 occurrences sorted by date, time, name; existing slots flagged | Default week = next week in `Asia/Jakarta` near midnight UTC; service with 14 times |
| TC-L-004 | Sequence filling | Song with arrangement `V1 C V2 C`; song without arrangement; song with no sections | Entries in that order; all sections in order; empty sequence | Arrangement entry deleted earlier is not present |
| TC-L-005 | Sequence validation | Section of another song; unknown part; key `H`, `Bb`; 100 and 101 entries; 10 and 11 songs | As in §2.3 | Same section many times is valid |
| TC-L-006 | Version rules | Each operation of §5 with the right, a stale and a missing version | Bumps as in the table; stale → conflict with `scope` | Item edit while the liturgy was reordered → accepted; reorder after an item was added → conflict |
| TC-L-007 | Item rules | Text on a song item; `reading_id` on a prayer; type change attempt; 60/61 items; insert position out of range | 422 / 422 / unknown field → 422 / limit / 422 | Insert at 0 and at n |
| TC-L-008 | Assignments | Member, non-member, free-text name, both, neither, duplicates (also `Name` vs `name`) | Rules of §2.4 | Removed member stays and is flagged |
| TC-L-009 | Problems | Liturgy with an empty song item, an unset reading, a deleted song and section | The five codes, none for a complete item | Reading item with deleted reading |
| TC-L-010 | History images | Each command of §7, including the worst legal case (10 songs × 100 entries; a template of 60 items with 5,000 characters) | Rows with before/after as in the table, `seq` rising by one per row, versions after, same transaction; the large images are stored and read back whole | A failing write leaves no row and no gap in `seq` |
| TC-L-011 | Language rules | Service `en` with a template `id`; request language `id` with the `en` template; with `template_id: ""`; prepare for such a service | 422 `language_mismatch`; 422; empty liturgy in `id`; the batch fails whole | Template without a language match deleted → none |
| TC-L-012 | Snapshots after deletion | Delete a song, a section and a reading used only by a published liturgy | `song_title`, `section_label`, `reading_label` remain; the GET shows them with `song: null` etc. and the `problems` codes | Editor can replace each removed reference |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-L-001 | Permissions and visibility | Church admin, liturgist, editor, team member, member of another church | Team member: liturgy list/get 404 for an unpublished liturgy, `assignable` 403; editor with `liturgy.edit` writes; a member with only `liturgy.comment` reads and gets 403 on writes; delete needs `liturgy.manage` (editor 403); other church → 404 | — |
| IT-L-002 | Create, edit, reorder | Template with 7 items | Create copies items; add, edit, reorder, remove with correct versions; each response carries the new versions | — |
| IT-L-003 | Concurrent editing | One liturgy, two sessions, items A and B | Both edit different items → both succeed; both edit A → second gets 409 `scope: "item"`; one reorders while the other adds an item → second gets 409 `scope: "liturgy"`; data unchanged by the refused write; 50 parallel edits of distinct items all succeed (race harness) | — |
| IT-L-004 | Prepare a week | Three services, two with existing liturgies | The listing flags existing slots; creating a mix with one real, one fake occurrence → 422 and nothing created; two concurrent prepares of the same slot create it once | — |
| IT-L-005 | Limits | `WithEntitlements` stub with `max_unpublished_liturgies = 2` | Third create → 403 `limit_reached` with `limit`, `used`, `max`; prepare of 2 with 1 slot left → 403 and nothing created; two concurrent creates take the last slot once | — |
| IT-L-006 | Usage rules | Draft liturgy using a song, section, reading | Song delete → 409 `song_in_use`; section removal → 409 `section_in_use` with IDs; reading delete → 409 `reading_in_use`; after the liturgy is deleted all succeed; a published liturgy (set directly in the database) does not block, and its references become null with snapshots kept | — |
| IT-L-007 | Locked states | Liturgy set to `in_review` directly | Every write route → 409 `liturgy_locked`; reads work; delete works for `liturgy.manage`; `published` delete → 409 `liturgy_not_deletable` | — |
| IT-L-008 | Isolation | Two churches | Church B cannot read or write church A's liturgies, items, assignments or edits (404), nor use A's songs, readings, duties or parts in its own liturgy (422) | — |
| IT-L-010 | Section removal race | Parallel `PATCH /songs/{id}` dropping a section and `PUT …/songs` adding it to a draft (race harness, both dialects) | Exactly one wins; either the section is gone and the entry never existed, or the `PATCH` gets 409 `section_in_use`; never a 500 or a dangling entry | — |
| IT-L-011 | Assignment and member removal | Parallel assignment of a member and removal of that membership | Either the assignment is refused (422) or it exists and is returned with `former_member: true`; never an assignment whose flag is stale | — |
| IT-L-012 | Write order | Two writers of one item from the same version; a failing child write | One 409 before any child row changes; the failing write leaves version, children and history unchanged | — |
| IT-L-009 | Constraints | Direct inserts bypassing the app | Two liturgies in one service slot, an assignment with both or neither of user and name, an item of another liturgy's church are rejected; deleting a liturgy removes its rows; an entry whose section belongs to another song is refused by the use case with 422, tested in package `app` (the database cannot express it, schema.md); deleting a song/section/reading through the repository sets the references to null (and a raw SQL delete is refused) | — |

Web tests are in [11 §8](11-liturgy-editor.md#8-test-case-specifications).

## 12. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| `validation_failed` | 422 with field list | "Check this field." | Fix and resubmit |
| `version_conflict` | 409, `scope`, `item_id?` | Item: "This item was changed by someone else. Your text is kept below." Liturgy: "The order of the items was changed by someone else." | Item reloads, own input kept; the page reloads the list |
| `liturgy_locked` | 409 | "This liturgy can't be edited now." | Reload |
| `liturgy_exists` | 409, `liturgy_id` | "There is already a liturgy for this service at this time." with a link | Open the existing one |
| `liturgy_not_deletable` | 409 | "A published liturgy can only be archived." | — |
| `assignment_exists` | 409 | "This person already has this duty." | — |
| `limit_reached` | 403 | "Your plan allows {{max}} liturgies. You have {{used}}." | Archive or delete one |
| `song_in_use`, `section_in_use`, `reading_in_use` | 409 | As in [06 §9](06-song-library.md#9-error-handling-matrix) and [07](07-readings.md) | — |
| `forbidden` | 403 | "You don't have permission to do this." | — |
| `not_found` | 404 | "This liturgy or item no longer exists." | Back to the list |

## 13. References

| Topic | Location |
|---|---|
| Weekly liturgy, review rules, versions | [SPEC.md §5.2](../SPEC.md#52-weekly-liturgy), [§5.5](../SPEC.md#55-review-workflow), [§11](../SPEC.md#11-decisions-log) |
| Tables | [reference/schema.md](../reference/schema.md#step-3-tables) |
| Atomic operations, locks | [02 §2.1](02-persistence.md#21-atomic-operations) |
| Authorization and `actions` | [04 §5](04-tenancy-extensions.md#5-authorization) |
| Entitlements | [04 §8](04-tenancy-extensions.md#8-entitlements) |
| Songs, arrangements, usage port | [06](06-song-library.md) |
| Readings, usage port | [07](07-readings.md) |
| Duties, singing parts, templates, services | [09](09-planning.md) |
| Pages, undo | [11](11-liturgy-editor.md) |
