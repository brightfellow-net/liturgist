# 06 — Song Library (Implementation)

> **Document type: Implementation.** Step 2 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slice 2A.
> Status: **Proposed** (draft 2026-10-02). Items marked **[P-xx]** are decisions listed in the [index](README.md#4-proposed-decisions).

## 1. Scope

Songs and their lyrics sections, default arrangements, links between language versions, copyright fields, search, and the library pages. Readings are in [07](07-readings.md); importing songs is in [08](08-import.md).

No entitlement limit applies to songs, readings or imports in step 2 ([SPEC.md §8.2](../SPEC.md#82-extensibility-and-editions)).

**Not in step 2** (they need liturgies, which arrive in step 3):

| Left out | Why | Where it lands |
|---|---|---|
| Usage history and the usage report (CSV) | No liturgy uses a song yet | Step 3 (history), step 5 (report) |
| The real "section in use" and "song in use" checks | Same | Step 3 fills the `SongUsage` port (§6); step 2 wires it and it always answers "unused" |
| Singing parts and sequences | Used only by liturgy items | Step 3 |
| Church licence settings (CCLI number, licence footer, credit lines on or off) | Used only when printing | Step 5 |
| Chords | SPEC §5.3 stores lyrics only | Not planned |

## 2. Data model

Domain types in `domain/song.go`; tables in [reference/schema.md](../reference/schema.md#songs). A church's songs are visible to its members only ([04 §5](04-tenancy-extensions.md#5-authorization)).

### 2.1 Song

| Field | Rule |
|---|---|
| `title` | 1–200 characters, trimmed |
| `alt_titles` | 0–10 entries, each 1–200 characters, trimmed, no duplicates (case-insensitive) |
| `language` | `id`, `en`, `zh-Hans` or `zh-Hant` (BCP 47, as for the church's content language) |
| `hymnal_source` | 0–40 characters, trimmed, e.g. `KJ`, `PKJ`, `NKB`; its **key** is upper-cased with spaces removed |
| `hymnal_number` | 0–10 characters, trimmed, e.g. `12`, `12a`; must be empty when `hymnal_source` is empty and vice versa. **Not unique** [P-53] |
| `lyricist`, `composer`, `translator` | 0–200 characters each |
| `default_key` | Empty or `^[A-G][#b]?m?$` (e.g. `G`, `Bb`, `F#m`) |
| `copyright_holder` | 0–200 characters |
| `copyright_line` | 0–300 characters; the exact text to print |
| `ccli_song_number` | Empty or 1–12 digits |
| `licence_status` | `unknown` (default), `public_domain`, `church_licence`, `permission_obtained` |
| `licence_notes` | 0–2000 characters |
| `default_arrangement` | Ordered list of this song's section IDs, 0–100 entries; the same ID may appear several times (V1, C, V2, C). Empty = "all sections in order" |
| `version` | Integer, 1 at creation, +1 on every change [P-46] |

Derived and stored for matching ([§5](#5-search)): `title_key` (the folded title, [§5.1](#51-folding)) and `hymnal_key` (`<source key>:<number lower-cased>`, null without a hymnal number).

### 2.2 Section

| Field | Rule |
|---|---|
| `id` | ULID, stable for the section's life: arrangements, and in step 3 liturgy sequences, refer to it |
| `kind` | `verse`, `pre_chorus`, `chorus`, `bridge`, `tag`, `intro`, `ending`, `other` |
| `number` | Required for `verse` (1–99), null for every other kind. Two verses of one song cannot share a number; numbers need not be contiguous |
| `label` | Null or 1–60 characters. Null means "derive from kind and number in the viewer's UI language" (`Verse 1`, `Bait 1`, `Chorus`, `Reff`); a custom label overrides it |
| `text` | 1–5000 characters after normalisation: `\r\n` and `\r` become `\n`, trailing spaces on every line are removed, leading and trailing blank lines are removed |

A song has at most 60 sections. Order is the array order of the API; `position` is dense (0, 1, 2 …).

### 2.3 Song groups

A group links the language versions of one hymn. It has no name in the UI. A group has **at least two songs and at most one song per language** (unique index, [schema](../reference/schema.md#songs)). Link and unlink run under `LockChurch` ([02 §3](02-persistence.md#3-connections-and-transactions)):

| Operation | Effect |
|---|---|
| `POST /songs/{id}/link {other_song_id}` | The same song twice → 422 `validation_failed`. Neither song grouped: new group with both. One grouped: the other joins that group. Both in the same group: no change (200). Both in different groups, or the joining song's language is already in the group → 409 `group_conflict` (`reason`: `already_grouped`, `language_taken`) |
| `DELETE /songs/{id}/link` | Removes the song from its group. A group left with one song is deleted and that song becomes ungrouped. Not grouped → 204, no change |

Changing a grouped song's `language` to one that another song of its group already has → 409 `group_conflict` (`reason`: `language_taken`).

### 2.4 Editing sections and concurrency [P-46]

- `PATCH /songs/{id}` takes the song's `version`. The update is conditional (`… WHERE id = $1 AND version = $2`, [02 §2.1](02-persistence.md#21-atomic-operations)); no row changed and the song exists → 409 `version_conflict`, nothing written. The response carries the new version.
- `sections`, when present, is the **complete ordered list**. An entry with an `id` of this song keeps that section (and may change its fields and position); an entry without `id` is new; a section whose `id` is absent from the list is **deleted**. An `id` of another song → 422.
- Deleting a section first asks `SongUsage.SectionsInUse` ([§6](#6-ports)); any section in use → 409 `section_in_use` with `section_ids`, nothing written. Step 2: never in use.
- In `POST` and `PATCH`, a **new** section carries a request-local `key` (1–40 characters, unique within the request) instead of an `id`. Entries of `default_arrangement` are existing section IDs or the `key`s of new sections in the same request, so one request can create sections and arrange them. The response contains the real IDs.
- Deleted sections are also removed from `default_arrangement`. An arrangement entry that names an unknown ID or `key` → 422.
- Everything above happens in one `Tx.Write`, including the search index update ([§5.3](#53-the-index)).

## 3. API

All paths under `/api/v1`. **View** = any member of the church; **edit** = `library.edit` ([03 §8](03-identity-auth.md#8-member-roles-and-permissions)). Responses carry `actions` ([04 §5](04-tenancy-extensions.md#5-authorization)).

| Method & path | Scope | Request | Response |
|---|---|---|---|
| `GET /songs` | view | query: `q`, `language`, `licence_status`, `hymnal_source`, `hymnal_number` (exact, with `hymnal_source`), `limit` (default 50, max 100), `offset` | `{ items: [SongSummary], total }` |
| `GET /songs/{id}` | view | — | Song with `sections`, `default_arrangement`, `versions` (the other songs in its group: `id`, `title`, `language`), `version`, `actions: { edit, delete }` |
| `POST /songs` | edit | all fields of §2.1 and `sections` | 201 Song |
| `PATCH /songs/{id}` | edit | `version` and any fields; `sections` as in §2.4 | Song |
| `DELETE /songs/{id}` | edit | — | 204; 409 `song_in_use` if `SongUsage.SongInUse` |
| `POST /songs/{id}/link` | edit | `{ other_song_id }` | Song |
| `DELETE /songs/{id}/link` | edit | — | 204 |

`SongSummary` = `id`, `title`, `alt_titles`, `language`, `hymnal_source`, `hymnal_number`, `licence_status`, `has_group`, `actions`. Lyrics are never in list responses. PATCH semantics follow [04 §6](04-tenancy-extensions.md#6-step-1-church-and-member-api): omitted = unchanged; strings that may be empty can be set to `""`.

Duplicate warning: the form asks `GET /songs?hymnal_source=KJ&hymnal_number=12` ("does this hymnal number exist already?"); the web app shows a warning with a link and saves anyway if the user insists [P-53].

Logging: song and section **text is never logged** (size, and copyright); logs carry IDs only. Info lines: `song_created`, `song_updated`, `song_deleted`, `songs_linked`, `songs_unlinked` (actor and song IDs).

## 4. Pages

All text through i18n keys ([05 §6](05-web-shell.md#6-translations)); tap targets and text size as in [05 §7](05-web-shell.md#7-accessibility-and-text-size). Menu item **Library** for every member (before **Settings**).

| Route | Page | Who |
|---|---|---|
| `/library` | Song list: search box, filters (language, licence status, hymnal source), results with title, hymnal number, language and licence status; paging. Empty library: a card explaining what to do with three actions: "Paste lyrics", "Import a file" ([08](08-import.md)), "Add a song". Readings are a second tab ([07](07-readings.md)) | View |
| `/library/songs/{id}` | The song: metadata, credit line, sections in order with their labels, the default arrangement, the other-language versions as links; "Edit" and "Delete" per `actions` | View |
| `/library/songs/new`, `/library/songs/{id}/edit` | Form: metadata in groups (Title and language; Hymnal; Credits; Licence), then the **sections editor**, then the default arrangement | `library.edit` |

Sections editor: one card per section with kind, number (verses), label (optional) and text; **Move up**, **Move down** and **Remove** buttons with text (no drag-only interaction, [P-52]); "Add section"; removing asks nothing until saved. After a `version_conflict` the page says the song changed meanwhile and offers "Reload" (the user's unsaved input is kept in the form until they choose). The arrangement editor lists the arrangement as a numbered list of section labels with Add (a select of the song's sections), Move up/down and Remove; "Use all sections in order" clears it. The language versions block has "Link another version…" (search by title, then choose) and "Unlink".

## 5. Search

### 5.1 Folding

`domain.Fold(s)` is the only normalisation, used for stored text and for queries: NFKC → lower-case → remove combining marks (NFD, drop `Mn`) → replace every run of characters that are not letters or digits with one space → trim. So `Besar Setia-Mu!` becomes `besar setia mu`, and `Tuhan, Engkau` becomes `tuhan engkau`. The databases' own tokenisers only ever see space-separated words, so SQLite and PostgreSQL return the same matches [P-48].

For `zh-Hans` and `zh-Hant` songs the folded text has **all** spaces removed (Chinese has no word boundaries).

### 5.2 Query semantics [P-48]

- The query is folded and split into terms; a song matches when **every term** matches.
- **Non-Chinese songs:** a term matches a word that **starts with** it, in the title, alternative titles, hymnal key or lyrics (full-text search). No stemming on either database.
- **Chinese songs:** a term (spaces removed) matches when it occurs **anywhere** in the folded title or lyrics (substring).
- **Hymnal query:** if the whole query looks like a hymnal reference (`^[a-z]+ ?\.?\d+[a-z]?$` after folding, e.g. `kj 12`, `pkj12a`), songs whose `hymnal_key` equals it are returned first.
- **Order:** (1) exact hymnal match; (2) every term matches the title, an alternative title or the hymnal text; (3) all other matches, where at least one term matches only in the lyrics. Inside each group by `title_key`, then `id`, compared bytewise (SQLite's default; `COLLATE "C"` on PostgreSQL), so both databases order identically; the same applies to the list order without a query.
- An empty query lists all songs by `title_key`. `q` is at most 200 characters, else 422.
- The filters combine with the query by AND.

Libraries are expected to hold hundreds to a few thousand songs; no result caching.

### 5.3 The index

Search reads `song_search`, a table owned by the songs repository and rewritten **in the same transaction** as every song, section or delete operation (delete the song's row, insert the new one):

| Column | Content |
|---|---|
| `church_id`, `song_id` | Keys; church-leading primary key |
| `language` | Copy of the song's language (filtering and the Chinese rule) |
| `title_fold` | Folded title and alternative titles joined by a space |
| `hymnal_fold` | Folded hymnal source and number |
| `lyrics_fold` | Folded section texts in order, joined by a space |

SQLite adds the FTS5 virtual table `song_fts` (`title`, `hymnal`, `lyrics`; `church_id` and `song_id` unindexed; tokenizer `unicode61`; `prefix='2 3'`), filled together with `song_search`. PostgreSQL adds a column `fts tsvector` on `song_search` (`to_tsvector('simple', title_fold || ' ' || hymnal_fold || ' ' || lyrics_fold)`, with weights for ordering) and a GIN index. Substring matching for Chinese uses `instr` (SQLite) or `strpos` (PostgreSQL) on the `_fold` columns. All of it lives behind the `SongSearch` port ([§6](#6-ports)) in the dialect layer.

`liturgist search reindex` rebuilds the whole index from the songs (exit 0; 1 on error). The virtual table has no foreign keys, so this is the repair tool, and a contract test (IT-S-006) checks that normal operations never need it.

## 6. Ports

In `app`, added to [02 §2](02-persistence.md#2-ports-in-app) and [04 §7](04-tenancy-extensions.md#7-extension-points):

| Port | Signature | Implementation | Used by |
|---|---|---|---|
| `ChurchStore.Songs()` | create, update (conditional on `version`), delete, `ByID`, `Search(ctx, SongQuery) (SongPage, error)`, `ReplaceSections`, `Link`, `Unlink`, `Reindex` | `adapters/sqlstore` | Song use cases |
| `SongUsage` | `SongInUse(ctx, church, song) (bool, error)`; `SectionsInUse(ctx, church, song, sections []SectionID) ([]SectionID, error)` | Step 2: `app.NeverUsed` (answers "unused"); step 3: queries the liturgies | Delete and section removal |

## 7. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Store lyrics as one text block | Ordered sections with kind and number | Slides, arrangements and sequences need sections (SPEC §5.3) |
| Identify a section by its position | By its stable `id` | Positions change on reordering; arrangements and liturgies refer to sections |
| Stem words or use a language-specific text-search configuration | `Fold` plus the `simple` configuration on both databases | Same results on SQLite and PostgreSQL; no Indonesian stemmer exists on SQLite |
| Update the search index in a later job or trigger | In the same transaction as the change | A crash must never leave a song unfindable or a deleted song findable |
| Log song or section text, or return lyrics in list responses | IDs in logs; lyrics only in `GET /songs/{id}` | Size and copyright |
| Write a song without checking `version` | Conditional update; 409 `version_conflict` | Two editors must not silently overwrite each other |
| Make the hymnal number unique | Warn on duplicates only | Churches use several hymnals and sometimes the same book twice (SPEC §5.3) |
| Block saving or printing because of `licence_status` | Record and filter only | SPEC §5.3: no enforcement |
| Ship sample songs or lyrics | Nothing: an empty library with helpful actions | SPEC §5.7: no sample data |

## 8. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-S-001 | Song validation | Each rule in §2.1 | Accepted and rejected examples per rule | Title of 200 and 201 characters; key `Bb`, `H`, `f#m`; hymnal number without source |
| TC-S-002 | Section validation | Each rule in §2.2 | As specified | Verse without number; chorus with a number; two verses numbered 1; 5000 and 5001 characters; `\r\n` text normalised |
| TC-S-003 | `Fold` | `"Besar Setia-Mu!"`, `"Cafè  Ünï"`, `"主，我愿意"` | `besar setia mu`, `cafe uni`, `主 我愿意` | Empty; only punctuation; full-width Latin letters |
| TC-S-004 | Hymnal key | `"KJ"` + `"12"`, `" pkj "` + `"12A"` | `KJ:12`, `PKJ:12a` | Source without number is rejected |
| TC-S-005 | Section replacement | Ordered list with kept, new and missing IDs; arrangement naming a new section by `key` | Kept IDs unchanged, new IDs created, missing deleted, positions dense; the arrangement holds the real IDs | Foreign ID → 422; unknown `key` → 422; duplicate `key` → 422; arrangement cleaned of deleted sections |
| TC-S-006 | Query parser | `"kj 12"`, `"PKJ12a"`, `"besar setia"`, `""` | Hymnal query; hymnal query; two terms; list all | 201-character query |
| TC-S-007 | Group rules | Link and unlink sequences | New group, join, same-group no-op, `already_grouped`, `language_taken`, group deleted at one member | Linking a song to itself; changing a grouped song's language to a taken one |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-S-001 | CRUD and permissions | Church admin, editor, team member | Team member can `GET` but not `POST`/`PATCH`/`DELETE` (403 `forbidden`); editor can; another church's song ID → 404 | — |
| IT-S-002 | Optimistic concurrency | One song, two sessions | Second `PATCH` with the old `version` → 409 `version_conflict`, data unchanged | — |
| IT-S-003 | Search, Indonesian and English | Songs in `id` and `en` | Prefix matches in title, hymnal number and lyrics; AND of terms; ordering by tier; diacritics ignored; identical results on SQLite and PostgreSQL | — |
| IT-S-004 | Search, Chinese | Songs in `zh-Hans` | One-character query finds lines containing it; query spanning a line break matches | — |
| IT-S-005 | Section removal | Song with arrangement `V1, C, V2, C` | Removing the chorus drops it from the arrangement; `SongUsage` stub answering "in use" → 409 `section_in_use`, nothing changed | — |
| IT-S-006 | Index consistency | Create, edit, delete, a failing transaction | `song_search` and `song_fts` match the songs after each, including after a rollback; `search reindex` changes nothing | — |
| IT-S-007 | Delete and groups | Grouped songs | Deleting a song removes its sections and index rows and dissolves a group left with one song; `SongUsage` "in use" → 409 `song_in_use` | — |
| IT-S-008 | Constraints | Direct inserts bypassing the app | Second song of one language in a group, a verse without number, a section of another church's song are rejected | — |

Web tests: unit tests for the sections editor (reordering and removing, `version_conflict` message) and the empty state; end-to-end tests E2E-W-007 (add a song with the form and two sections, find it by a lyric word and by hymnal number) and E2E-W-008 (edit a song, reorder sections, save, reload); axe covers the new pages (E2E-W-005 is extended).

## 9. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| `validation_failed` | 422 with field list | "Check this field." on the fields | Fix and resubmit |
| `version_conflict` | 409 | "This song was changed by someone else. Reload to see their version." | Reload button; unsaved input kept |
| `section_in_use` | 409 | "A section is used in a liturgy that isn't published yet, so it can't be removed." | Stay on page |
| `song_in_use` | 409 | "This song is used in a liturgy that isn't published yet, so it can't be deleted." | Stay on page |
| `group_conflict` | 409, `reason` | `already_grouped`: "One of these songs is already linked to other versions. Unlink it first." `language_taken`: "There is already a version in this language." | Choose another song |
| `forbidden` | 403 | "You don't have permission to do this." | Stay on page |
| `not_found` | 404 | "Not found." | Back to the library |
| Search backend error | 500/503 | "Search isn't working right now. Please try again." | Retry button |

## 10. References

| Topic | Location |
|---|---|
| Song library, copyright, search decisions | [SPEC.md §5.3](../SPEC.md#53-song-library), [§11](../SPEC.md#11-decisions-log) |
| Tables | [reference/schema.md](../reference/schema.md#songs) |
| Atomic operations, locks | [02 §2.1](02-persistence.md#21-atomic-operations), [02 §3](02-persistence.md#3-connections-and-transactions) |
| Authorization and `actions` | [04 §5](04-tenancy-extensions.md#5-authorization) |
| Scopes | [03 §8](03-identity-auth.md#8-member-roles-and-permissions) |
| Web shell, accessibility | [05](05-web-shell.md) |
