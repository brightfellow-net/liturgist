# 08 — Importing Songs (Implementation)

> **Document type: Implementation.** Step 2 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slice 2C.
> Status: **Proposed** (draft 2026-10-02). Items marked **[P-xx]** are decisions listed in the [index](README.md#4-proposed-decisions).

## 1. Scope

Getting existing songs into the library quickly ([SPEC.md §5.3](../SPEC.md#53-song-library), "Importing existing content"): **paste and split**, **OpenLyrics** (OpenLP) and **ChordPro** files, the batch and candidate review step, duplicate detection, and the `Importer` port. Imported content is the church's own material in its own install.

Not in step 2: EasyWorship and PowerPoint importers (pilot phase; they read binary files, which need a decision about the JSON-only rule of [03 §6](03-identity-auth.md#6-csrf-protection) first), Word and spreadsheet imports, AI-assisted import, and importing readings (the candidate table allows `kind = 'reading'`, but no step-2 format produces one; Bible-file import comes after the MVP).

## 2. Flow [P-51]

1. The member (scope `library.edit`) chooses a format and sends the text. The server parses it with the matching `Importer` and stores an **import batch** with its **candidates**; the response lists them.
2. The review page shows each candidate as a song preview with its warnings and, if found, the existing song it may duplicate. The member edits the draft, and decides per candidate: **accept** (new song), **merge** (into an existing song), or **skip**. "Accept all" accepts every candidate that has no duplicate; candidates with a duplicate stay undecided.
3. **Apply** creates the songs. Each candidate is applied in its own `Tx.Write` through the normal song use cases ([06](06-song-library.md)), so validation, church scoping and search indexing are identical to typing a song in; one failing candidate does not stop the others. The transaction starts with `LockChurch` ([02 §3](02-persistence.md#3-connections-and-transactions)) and re-reads the candidate: one that is already applied is skipped. So applying again, or two simultaneous Applies, create every song exactly once.
4. Batches are working data: a batch that is closed, discarded, or untouched for 7 days is deleted by the cleanup job ([03 §11](03-identity-auth.md#11-cleanup-job)), with its candidates.

Nothing is saved to the library before **Apply**.

## 3. Data

Domain types `domain.ImportBatch`, `domain.ImportCandidate`, `domain.SongDraft`; tables in [reference/schema.md](../reference/schema.md#import_batches).

| Item | Rule |
|---|---|
| Batch `source_format` | `paste`, `openlyrics`, `chordpro` (later `easyworship`, `pptx`) |
| Batch `status` | `open`, `closed`. Closed by **Apply** when no candidate is `pending` any more, or by discarding (then deleted at once) |
| `SongDraft` | The fields of a song create request ([06 §2](06-song-library.md#2-data-model)) with `sections` as a list without IDs, and `default_arrangement` as indexes into `sections` |
| Candidate `decision` | `pending` (default), `accept`, `merge`, `skip` |
| `merge_into` | Song ID, required with `merge`; must belong to this church |
| `duplicate_of_id` | The first existing song of this church with the same `hymnal_key`, else the same `title_key` and language; null if none. Computed when the batch is created and again on `GET`, so a song deleted meanwhile no longer counts. Not a foreign key |
| `applied_song_id`, `error_code` | Set by **Apply**: the created or merged song, or the error code of a failure |
| `warnings` | Codes the parser produced (§4.1), plus `duplicate_in_batch` when two candidates of one batch share a `hymnal_key`, or a folded title and language (both stay `pending`); each has a translated message |

### Merge

Merge changes the existing song as follows:

- **Metadata:** every field that is empty in the existing song is filled from the draft; a field that already has a value is never overwritten. Alternative titles are united.
- **Sections:** replaced by the draft's sections. A draft section takes over the **ID** of an existing section with the same kind and number (verses) or the same kind (other kinds, first unused match in order), so references to it stay valid; unmatched draft sections get new IDs; existing sections that match nothing are deleted, subject to `SongUsage` ([06 §2.4](06-song-library.md#24-editing-sections-and-concurrency-p-46)) — a section in use makes that candidate fail with `section_in_use`.
- **Default arrangement:** the draft's if it has one, otherwise the existing one cleaned of deleted sections.
- The song's `version` increases by one.
- If the song to merge into was deleted after the decision, the candidate fails with `not_found`.

## 4. Formats

Every parser is a pure function of its text, in `adapters/importers/<format>`, implementing `Importer.Parse(ctx, r io.Reader, hint ImportHint) ([]ImportCandidate, error)`. `ImportHint` = `Format`, `Language` (BCP 47, default: the church's content language), `Name` (file name, or the title for paste). Text is read as UTF-8 (a BOM is removed; invalid bytes → `import_unreadable`). A parser never writes to the database and never fetches anything.

### 4.1 Paste and split

The user types the **title** (required) and pastes the lyrics. Rules, applied in order:

1. Line endings become `\n`; text is split into **blocks** at one or more blank lines (lines with only white space).
2. If a block's first line is a **label** (below), the label sets the section kind and number and is removed from the text. A label may be followed on the same line by lyrics (`1. Besar setia-Mu…`): the rest of the line stays as the first line of the text. A block with only a label line that **repeats** an earlier labelled section (`Reff` alone) adds that section to the arrangement again and creates no new section.
3. **Unlabelled** blocks are verses, numbered in order after the labelled verses' numbers (1, 2, 3 …); warning `blocks_numbered`.
4. **Repeated chorus guess:** when the same unlabelled block (compared folded, [06 §5.1](06-song-library.md#51-folding)) appears two or more times, it becomes one `chorus` section and its copies are dropped; warning `chorus_guessed`. If it equals an already labelled section, the copies are dropped without a warning.
5. Whenever blocks were dropped or labels repeated, the order of appearance becomes the draft's `default_arrangement` (for example verse 1, chorus, verse 2, chorus). Without repeats there is no arrangement.
6. Verse numbers that appear twice are renumbered in order (warning `verse_renumbered`); a number on a non-verse label is dropped (warning `number_dropped`); blocks that are empty after normalisation are ignored; more than 60 sections → `import_unreadable` for that candidate.

Labels are matched case-insensitively on the first line, ignoring a trailing `:` or `.`; a number may follow:

| Kind | Indonesian | English | Chinese |
|---|---|---|---|
| `verse` | `Bait 1`, `Ayat 1`, `1.`, `1)`, `1:` | `Verse 1`, `V1`, `1.` | `第一节`, `第1节`, `一、`, `1.` |
| `chorus` | `Reff`, `Ref`, `Refrein` | `Chorus`, `Refrain` | `副歌` |
| `pre_chorus` | `Pra-Reff`, `Pre-Reff` | `Pre-Chorus`, `Prechorus`, `Pre Chorus` | `前副歌`, `导歌` |
| `bridge` | `Jembatan` | `Bridge` | `桥段` |
| `tag` | | `Tag` | |
| `intro` | `Intro` | `Intro` | `前奏` |
| `ending` | `Akhir` | `Outro`, `Ending`, `Coda` | `尾声` |
| `other` | `Interlude`, `Musik` | `Interlude`, `Instrumental` | `间奏` |

Chinese numerals 一 to 十 are read as 1 to 10. The table is data in `adapters/importers/paste/labels.go`.

### 4.2 OpenLyrics (OpenLP)

One song per file. The file is XML (`encoding/xml`; no DTD or external entity is ever resolved).

| OpenLyrics | Draft |
|---|---|
| `properties/titles/title` (first = title, others = alternative titles) | `title`, `alt_titles` |
| `properties/authors/author` with `type` `words`, `music`, `translation` (no type = lyricist) | `lyricist`, `composer`, `translator` (several authors joined with `, `) |
| `properties/copyright` | `copyright_line` |
| `properties/ccliNo` | `ccli_song_number` (digits only, else warning `ccli_ignored`) |
| `properties/songbooks/songbook` (`name`, `entry`) — the first | `hymnal_source`, `hymnal_number` |
| `properties/key` | `default_key` if it fits the key rule, else warning `key_ignored` |
| `lyrics/verse` `name`: `v1` verse 1, `c`/`c1` chorus, `p` pre-chorus, `b` bridge, `i` intro, `e` ending, `o`/other other | `sections` with kind and number; the text is the `lines` with `<br/>` as line breaks and all markup (chords, formatting tags) removed |
| `properties/verseOrder` (space-separated verse names) | `default_arrangement` (a name that doesn't exist → warning `arrangement_ignored`, no arrangement) |

### 4.3 ChordPro

A file may hold several songs separated by `{new_song}`. Chords in `[brackets]` are removed (chords are not stored).

| ChordPro | Draft |
|---|---|
| `{title: …}` / `{t: …}`; `{subtitle: …}` / `{st: …}` (alternative title) | `title`, `alt_titles` |
| `{lyricist: …}`, `{composer: …}`, `{key: …}`, `{ccli: …}`, `{copyright: …}`; `{meta: songbook …}` / `{meta: number …}` | the matching fields |
| `{start_of_verse}`/`{sov}`, `{start_of_chorus}`/`{soc}`, `{start_of_bridge}`/`{sob}` … `{end_of_…}`; a label argument (`{sov: Verse 2}`) is read with the table of §4.1 | `sections` |
| Lines outside any block | Split by §4.1 rules 1–3 as one more paste |
| Comments (`# …`), other directives | Ignored |

### 4.4 Errors

A file or text that cannot be read at all → 422 `import_unreadable` with `reason`: `not_utf8`, `not_xml`, `no_song`, `too_many_sections`. In a request with several files, unreadable files are reported in `rejected: [{name, reason}]` and the others still produce candidates; if nothing produced a candidate the response is 422.

## 5. API

All paths under `/api/v1`; every operation needs `library.edit`. Only the creator's church can see its batches (404 otherwise).

| Method & path | Request | Response |
|---|---|---|
| `POST /imports` | `{ format, language?, files: [{ name, text }] }` — `paste`: exactly one file whose `name` is the title | 201 `{ id, status, candidates: [Candidate], rejected: [...] }` |
| `GET /imports/{id}` | — | The batch with its candidates |
| `PATCH /imports/{id}/candidates/{cid}` | `{ draft }` (a complete `SongDraft`, validated as in [06 §2](06-song-library.md#2-data-model)) | The candidate; `duplicate_of` recomputed |
| `PATCH /imports/{id}/candidates` | `{ decisions: [{ id, decision, merge_into? }] }` (at most 500) | The candidates |
| `POST /imports/{id}/apply` | — | `{ created, merged, skipped, failed: [{ candidate_id, code }] }`; status `closed` when nothing is `pending` |
| `DELETE /imports/{id}` | — | 204 (batch and candidates deleted) |

`Candidate` = `id`, `draft`, `duplicate_of: { id, title, hymnal_source, hymnal_number } | null`, `decision`, `merge_into`, `warnings`, `applied_song_id`, `error_code`.

**Limits [P-51]:** the body of `POST /imports` may be up to **6 MiB** (every other operation keeps the 1 MiB of [01 §8](01-foundation.md#8-http-basics); over the limit → 413 `validation_failed`). Pasted lyrics: at most 200,000 characters (422 on the field). At most 200 files per request, 500 candidates per batch (more → 422 `validation_failed`). Text is sent as a JSON string, so the rule of [03 §6](03-identity-auth.md#6-csrf-protection) is untouched; the browser reads files with `File.text()`.

Logging: info lines `import_created` (batch ID, format, number of candidates), `import_applied` (batch ID, counts). Lyrics are never logged.

## 6. Pages

| Route | Page |
|---|---|
| `/library/import` | Step 1: three choices — "Paste lyrics" (title, language, text box), "OpenLyrics files" and "ChordPro files" (file picker, several files allowed). Text says what the app accepts and that nothing is saved yet |
| `/library/import/{id}` | Step 2, review: a list of candidates (title, number of sections, warnings in words, duplicate notice with a link to the existing song); a preview of the selected candidate with its sections and an editor like [06 §4](06-song-library.md#4-pages); per candidate a choice **Add as new song**, **Merge into "…"** (offered with the duplicate preselected as a suggestion, never as a default), **Skip**; buttons "Add all without duplicates" and "Import chosen songs". After **Apply**: a summary and links to the new songs; failures are listed with their reason |

The library's empty state links here ([06 §4](06-song-library.md#4-pages)). Navigating away from a review keeps the batch for 7 days; the library page shows "You have an unfinished import" with a link while an `open` batch of the member's church exists.

## 7. Ports

| Port | Signature | Implementation | Used by |
|---|---|---|---|
| `Importer` | `Parse(ctx, r io.Reader, hint ImportHint) ([]ImportCandidate, error)` (exists since step 1, [04 §7](04-tenancy-extensions.md#7-extension-points)) | `adapters/importers/paste`, `…/openlyrics`, `…/chordpro`, registered by `server` in a map keyed by format; `server.WithImporter(format, Importer)` adds or replaces one | Import use case |
| `ChurchStore.Imports()` | batches and candidates: create, get, update draft, set decisions, mark applied, delete, delete older than | `adapters/sqlstore` | Import use case, cleanup job |

`app.ImportHint` and `app.ImportCandidate` stop being placeholders in this step (listed under **Ports** in `CHANGELOG.md`).

## 8. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Save imported songs before the review | Parse into candidates; save on **Apply** | SPEC §5.3: the user checks the result first |
| Write imported songs with direct SQL | Apply through the song use cases | Validation, church scoping and the search index must be the same as typing |
| Make one failing candidate roll back the whole batch | One transaction per candidate; report failures | One bad file shouldn't lose 49 good songs |
| Overwrite filled fields when merging | Fill empty fields only | The church's own edits are worth more than a re-import |
| Resolve XML entities or DTDs, or fetch anything while parsing | Plain `encoding/xml`, no network | Hostile files |
| Trust the import's sizes, labels or numbers | The same validation as for typed songs, plus the limits of §5 | Imported data is untrusted input |
| Guess silently | Add a warning code for every guess | The user must see what was assumed |
| Log or return lyrics in logs and error messages | IDs and counts only | Size and copyright |

## 9. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-I-001 | Paste and split | Four blocks: three verses, a repeating unlabelled chorus after each | Three verses, one chorus, arrangement V1 C V2 C V3 C, warning `chorus_guessed` | No repeats → no arrangement; `\r\n`; tabs and trailing spaces; leading blank lines |
| TC-I-002 | Labels | Every label of §4.1 in the three languages | Right kind and number; label removed from the text | `Bait 1:` with text on the same line; `Reff` alone later → arrangement entry only; `Chorus 2` → `number_dropped`; `第二节`; unknown label stays text |
| TC-I-003 | Verse numbering | `1.` `3.` unlabelled; duplicate `1.` | Numbers kept / renumbered with `verse_renumbered` | More than 60 blocks → `too_many_sections` |
| TC-I-004 | OpenLyrics | The OpenLyrics 0.8 sample song and a song with `verseOrder`, three author types, a songbook, markup in lines | Every mapping of §4.2 | Missing title → `no_song`; invalid key → `key_ignored`; unknown verse name; DOCTYPE with an entity is not expanded |
| TC-I-005 | ChordPro | A song with chords, `{sov}`/`{soc}`, `{subtitle}`, `{key}`; a file with two songs | Chords removed, sections and metadata mapped, two candidates | Lines outside blocks; comments; `{new_song}` first line |
| TC-I-006 | Duplicate detection | Candidates equal to an existing song by hymnal key, by title and language; two equal candidates in one batch | `duplicate_of` set; same title in another language → none; `duplicate_in_batch` on both | `Besar Setia-Mu` equals `besar setia mu` after folding (duplicate), but not `besar setiamu` (no duplicate) |
| TC-I-007 | Merge | Existing song with edited metadata and three sections; draft with four | Filled empty fields only; IDs kept for matching kind/number; extra existing sections deleted; version +1 | Section in use (stub) → `section_in_use` |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-I-001 | Paste, review, apply | Editor | Batch created, nothing in the library; accept → song exists with the sections and is found by search | — |
| IT-I-002 | Partial failure | Batch with one candidate made invalid after creation (e.g. title edited to 201 characters directly in the database) | Others applied, failure reported with its code, batch stays `open` | — |
| IT-I-003 | Idempotent apply | Applied batch; a second batch | A second `apply` creates nothing; two simultaneous Applies of one batch (race harness, [02 §2.1](02-persistence.md#21-atomic-operations)) create each song once, on both dialects | — |
| IT-I-004 | Decisions | Duplicates and non-duplicates | "Accept all" leaves duplicates `pending`; `merge` without `merge_into` or with another church's song → 422 / 404 | — |
| IT-I-005 | Limits and permissions | Team member; oversize body; 201 files | 403; 413 `validation_failed`; 422 | — |
| IT-I-006 | Isolation | Two churches | Church B cannot read, change, apply or delete church A's batch (404) | — |
| IT-I-007 | Cleanup | Open batch older than 7 days, closed batch | Deleted by the cleanup job with their candidates; a recent open batch stays | — |

Web tests: unit tests for the review page (decisions, duplicate suggestion never preselected, "Add all without duplicates"); end-to-end tests E2E-W-010 (paste lyrics, review, import, find the song) and E2E-W-011 (import an OpenLyrics and a ChordPro file); axe covers both import pages.

## 10. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| `import_unreadable` | 422, `reason` | `not_utf8`: "This file isn't plain text in UTF-8." `not_xml`: "This doesn't look like an OpenLyrics file." `no_song`: "No song was found." `too_many_sections`: "This song has more than 60 sections." | Choose another file |
| Body too large | 413 `validation_failed` | "That is too much at once. Import fewer files." | Split the files |
| `validation_failed` on a draft | 422, field list | "Check this field." | Edit the draft |
| `section_in_use` (merge) | per-candidate `error_code` | "A section of the song to merge into is used in a liturgy that isn't published yet." | Skip or choose accept |
| `not_found` | 404 | "This import no longer exists. Start again." | Back to the import page |
| `forbidden` | 403 | "You don't have permission to do this." | — |

## 11. References

| Topic | Location |
|---|---|
| Import decisions | [SPEC.md §5.3](../SPEC.md#53-song-library), [§11](../SPEC.md#11-decisions-log) |
| `Importer` port | [04 §7](04-tenancy-extensions.md#7-extension-points) |
| Songs, folding, merge rules | [06](06-song-library.md) |
| Tables | [reference/schema.md](../reference/schema.md#import_batches) |
| Cleanup job | [03 §11](03-identity-auth.md#11-cleanup-job) |
