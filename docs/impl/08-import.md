# 08 — Importing Songs (Implementation)

> **Document type: Implementation.** Step 2 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slice 2C.
> Status: **Approved** 2026-10-03 (drafted 2026-10-02; Spec Gate and adversarial review round 3 done). Items marked **[P-xx]** are decisions listed in the [index](README.md#4-proposed-decisions).

## 1. Scope

Getting existing songs into the library quickly ([SPEC.md §5.3](../SPEC.md#53-song-library), "Importing existing content"): **paste and split**, **OpenLyrics** (OpenLP) and **ChordPro** files, the batch and candidate review step, duplicate detection, and the `Importer` port. Imported content is the church's own material in its own install.

Not in step 2: EasyWorship and PowerPoint importers (pilot phase; they read binary files, which need a decision about the JSON-only rule of [03 §6](03-identity-auth.md#6-csrf-protection) first), Word and spreadsheet imports, AI-assisted import, and importing readings (the candidate table allows `kind = 'reading'`, but no step-2 format produces one; Bible-file import comes after the MVP).

## 2. Flow [P-51]

1. The member (scope `library.edit`) chooses a format and sends the text. The server parses it with the matching `Importer` and stores an **import batch** with its **candidates**; the response lists them.
2. The review page shows each candidate as a song preview with its warnings and, if found, the existing song it may duplicate. The member edits the draft, and decides per candidate: **accept** (new song), **merge** (into an existing song, after seeing exactly what changes, [§3.2](#32-merge)), or **skip**. "Accept all" accepts every candidate that has no duplicate; candidates with a duplicate stay undecided.
3. **Apply** creates the songs. Each candidate is applied in its own `Tx.Write` through the normal song use cases ([06](06-song-library.md)), so validation, church scoping and search indexing are identical to typing a song in; one failing candidate does not stop the others. The transaction starts with `LockChurch` ([02 §3](02-persistence.md#3-connections-and-transactions)) and re-reads the candidate under the lock. **Every candidate mutation (editing a draft, setting a decision) takes the same lock and re-checks the candidate's state**, so Apply and a reviewer's edit are serialised and neither works on stale data. Applying again, or two simultaneous Applies, create every song exactly once.
4. Batches are working data: a batch that is closed, discarded, or untouched for 7 days is deleted by the cleanup job ([03 §11](03-identity-auth.md#11-cleanup-job)), with its candidates.

Nothing is saved to the library before **Apply**.

### 2.1 States

A candidate has a `decision` (what the member chose) and an `outcome` (what Apply did). A batch has a `status`.

| `decision` | `outcome` | Meaning | Terminal? |
|---|---|---|---|
| `pending` | null | Not decided yet | No |
| `accept` or `merge` | null | Chosen, not applied yet | No |
| `accept` or `merge` | `failed` (with `error_code`) | Apply tried and failed; **retryable**: the next Apply tries it again, and the member may edit the draft or change the decision first | No |
| `accept` or `merge` | `applied` (with `applied_song_id`) | Done | Yes |
| `skip` | null | Left out | Yes |

- A candidate whose outcome is `applied` cannot be edited or re-decided: `PATCH` → 409 `import_conflict` (`reason`: `already_applied`). A `failed` candidate can be edited and re-decided, which clears its outcome.
- Batch `status` is `open` until **every candidate is terminal**; Apply then sets it to `closed` in the same transaction as the last candidate. A batch with a failed or undecided candidate stays `open`. Discarding deletes the batch at once.
- Changing the decision of a candidate in a `closed` batch (for example a skipped one) makes the batch `open` again.
- There is no "applying" state: each candidate is applied inside one short transaction under the church lock, so a second Apply simply sees `applied` and skips it.

## 3. Data

Domain types `domain.ImportBatch`, `domain.ImportCandidate`, `domain.SongDraft`; tables in [reference/schema.md](../reference/schema.md#import_batches).

| Item | Rule |
|---|---|
| Batch `source_format` | `paste`, `openlyrics`, `chordpro` (later `easyworship`, `pptx`) |
| Batch `status` | `open`, `closed` ([§2.1](#21-states)) |
| `SongDraft` | The fields of a song create request ([06 §2](06-song-library.md#2-data-model)) with `sections` as a list without IDs, and `default_arrangement` as indexes into `sections` |
| Candidate `decision` / `outcome` | [§2.1](#21-states) |
| `merge_into` | Song ID, required with `merge`; must belong to this church |
| `merge_target_version` | The `version` of the `merge_into` song when the decision was saved (a version that is not the current one is refused when saving, 409 `import_conflict` / `target_changed`); Apply compares it with the song's current version and fails with `import_conflict` (`reason`: `target_changed`) if it differs. The member then reopens the preview, which shows the song as it is now, and saves the decision again |
| `remove_unmatched` | Boolean, default false; meaningful only with `merge` ([§3.2](#32-merge)) |
| `duplicate_of_id` | The first existing song of this church with the same `hymnal_key`, else the same `title_key` and language; null if none. Computed when the batch is created and again on `GET`, so a song deleted meanwhile no longer counts. Not a foreign key |
| `applied_song_id`, `error_code` | Set by Apply with `outcome`. `error_code` is one of `validation_failed`, `section_in_use`, `not_found`, `target_changed` |
| `warnings` | Codes the parser produced (§4), plus `duplicate_in_batch` when two candidates of one batch share a `hymnal_key`, or a folded title and language (both stay `pending`); the batch warning is computed whenever a batch is read and is not stored; each has a translated message |

### 3.2 Merge

Merge changes an existing song, so the review page must show a **preview before the member can choose it** and Apply does exactly what the preview showed:

- **Metadata:** every field that is empty in the existing song is filled from the draft; a field that already has a value is never overwritten. Alternative titles are united.
- **Section matching** is deterministic, so no ID is reused where identity is uncertain:
  1. Verses match by number.
  2. For every other kind, a draft section and an existing section match when **exactly one** of that kind exists on each side.
  3. If a kind occurs more than once on either side, sections of that kind match only when their folded texts are **identical**; if the counts are equal and no texts are identical, they do **not** match.
  4. A matched draft section takes the existing section's **ID** and replaces its text (and label); an unmatched draft section is **new** (new ID).
- **Unmatched existing sections** are **kept** unless the member ticks "Remove the N sections that are not in the import" (`remove_unmatched = true`). Removing is subject to `SongUsage` ([06 §2.4](06-song-library.md#24-editing-sections-and-concurrency-p-46)): a section in use makes that candidate fail with `section_in_use`. Kept sections stay after the matched and new ones, in their old order.
- **Default arrangement:** the draft's, if it has one, with entries pointing at the matched or new section IDs; otherwise the existing one (entries of removed sections are dropped by the database).
- The song's `version` increases by one.
- If the song to merge into was deleted after the decision, the candidate fails with `not_found`.
- The preview lists every section of the result with its status: **updated** (with the old and new text side by side), **new**, **kept** or **removed**.

## 4. Formats

Every parser is a pure function of its text, in `adapters/importers/<format>`, implementing `Importer.Parse(ctx, r io.Reader, hint ImportHint) ([]ImportCandidate, error)`. `ImportHint` = `Format`, `Language` (BCP 47, default: the church's content language), `Name` (file name, or the title for paste). Text is read as UTF-8 (a BOM is removed; invalid bytes → `import_unreadable`). A parser never writes to the database and never fetches anything.

### 4.1 Paste and split

The user types the **title** (required) and pastes the lyrics. Rules, applied in order:

1. Line endings become `\n`; text is split into **blocks** at one or more blank lines (lines with only white space).
2. If a block's first line is a **label** (below), the label sets the section kind and number and is removed from the text. A label may be followed on the same line by lyrics (`1. Besar setia-Mu…`): the rest of the line stays as the first line of the text. A block with only a label line that **repeats** an earlier labelled section (`Reff` alone) adds that section to the arrangement again and creates no new section.
3. **Unlabelled** blocks are verses, numbered in order after the labelled verses' numbers (1, 2, 3 …); warning `blocks_numbered`.
4. **Repeats:** an unlabelled block whose folded text ([06 §5.1](06-song-library.md#51-folding)) equals an **earlier section** (labelled, or an earlier unlabelled block) is a *repeat* of that section and creates no new section. When an unlabelled block that is not a copy of an earlier section occurs **two or more times**, its first occurrence becomes one `chorus` section (warning `chorus_guessed`) and the later occurrences are repeats of it.
5. **Arrangement:** if at least one repeat occurred (rules 2 and 4), the draft gets a `default_arrangement` listing **every occurrence in order of appearance**, first occurrences and repeats alike, each pointing at its section. Without a repeat there is no arrangement.
6. Verse numbers that appear twice are renumbered in order (warning `verse_renumbered`); a number on a non-verse label is dropped (warning `number_dropped`); blocks that are empty after normalisation are ignored; more than 60 sections → `import_unreadable` for that candidate.

Worked examples (blocks separated by blank lines; `⏎` is a line break):

| Input blocks | Sections (in order) | Arrangement | Warnings |
|---|---|---|---|
| `a` / `x` / `b` / `x` / `c` / `x` (all unlabelled) | verse 1 = `a`, chorus = `x`, verse 2 = `b`, verse 3 = `c` | verse 1, chorus, verse 2, chorus, verse 3, chorus | `chorus_guessed`, `blocks_numbered` |
| `1. a` / `Reff⏎x` / `2. b` / `Reff` / `3. c` / `Reff` | verse 1, chorus = `x`, verse 2, verse 3 | verse 1, chorus, verse 2, chorus, verse 3, chorus | none |
| `Verse 1⏎a` / `Chorus⏎x` / `Verse 2⏎b` / `x` (unlabelled copy of the chorus) | verse 1, chorus, verse 2 | verse 1, chorus, verse 2, chorus | none |
| `a` / `b` / `c` | verses 1–3 | none | `blocks_numbered` |

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
| `lyrics/verse` `name`: `v1` verse 1, `c`/`c1` chorus, `p` pre-chorus, `b` bridge, `i` intro, `e` ending, `o`/other other. A number on a non-verse name is dropped (`number_dropped`); a verse number that appears twice or is missing is renumbered in order (`verse_renumbered`); author types other than the three listed are ignored | `sections` with kind and number; the text is the `lines` with `<br/>` as line breaks and all markup (chords, formatting tags) removed |
| `properties/verseOrder` (space-separated verse names) | `default_arrangement` (a name that doesn't exist → warning `arrangement_ignored`, no arrangement) |

### 4.3 ChordPro

A file may hold several songs separated by `{new_song}`. Chords in `[brackets]` are removed (chords are not stored).

| ChordPro | Draft |
|---|---|
| `{title: …}` / `{t: …}`; `{subtitle: …}` / `{st: …}` (alternative title) | `title`, `alt_titles`. Without a title the file name (without extension) is the title, warning `title_from_file` |
| `{lyricist: …}`, `{composer: …}`, `{key: …}`, `{ccli: …}`, `{copyright: …}`; `{meta: songbook …}` / `{meta: number …}` | the matching fields |
| `{start_of_verse}`/`{sov}`, `{start_of_chorus}`/`{soc}`, `{start_of_bridge}`/`{sob}` … `{end_of_…}`; a label argument (`{sov: Verse 2}`) is read with the table of §4.1 | `sections` |
| Lines outside any block | Split by §4.1 rules 1–3 as one more paste |
| `# …` comment lines | Ignored silently (not part of the song) |
| `{capo}`, `{tempo}`, `{time}`, `{duration}`, `{columns}`, `{column_break}`, `{new_page}`, `{pagetype}`, `{define}`, `{chord}`, the font, size and colour directives (`{textfont}`, `{textsize}`, `{textcolour}`, `{chordfont}`, `{chordsize}`, `{chordcolour}`), `{image}`, and `{meta}` keys not listed above | Ignored silently (no lyrics, no structure) |
| `{comment}`/`{c}`, `{comment_italic}`/`{ci}`, `{comment_box}`/`{cb}`, `{highlight}` | Text dropped, warning `comment_ignored` |
| `{start_of_tab}`/`{sot}` … `{end_of_tab}` and `{start_of_grid}` … `{end_of_grid}` | Content dropped, warning `block_ignored` |
| Any other directive | Ignored, warning `directive_ignored` (once per directive name per song) |

### 4.4 Errors and failure granularity

- A file or text that cannot be read at all is rejected with 422 `import_unreadable` and a `reason`: `not_utf8`, `not_xml`, `no_song`, `too_many_sections`, `file_too_large`, `too_complex` ([§5](#5-api) gives the limits).
- A **multi-song file** (ChordPro) is first split at `{new_song}`; each song is parsed **independently**. A song that is malformed or exceeds a limit becomes an entry in `rejected` (`name`, `song_index`, `reason`); the other songs of the file still produce candidates.
- In a request with several files, each unreadable file is rejected on its own in the same way. If nothing produced a candidate the response is 422 `import_unreadable` with the reason of the first rejection; otherwise it is 201 and lists `rejected`.

## 5. API

All paths under `/api/v1`; every operation needs `library.edit`. **Authorization:** every `/imports` route needs `library.edit`, reading included (403 `forbidden` otherwise, also after a member loses the scope); every member of the church holding it sees all of the church's batches, not only their own; other churches' batches are 404.

| Method & path | Request | Response |
|---|---|---|
| `POST /imports` | `{ format, language?, files: [{ name, text }] }` — `paste`: exactly one file whose `name` is the title | 201 `{ id, status, candidates: [Candidate], rejected: [...] }` |
| `GET /imports` | — | `{ items: [{ id, source_format, created_at, updated_at }] }`: the church's batches that are `open`, newest first (for the library page's "unfinished import" notice, [§6](#6-pages)) |
| `GET /imports/{id}` | — | The batch with its candidates |
| `PATCH /imports/{id}/candidates/{cid}` | `{ draft }` (a complete `SongDraft`, validated as in [06 §2](06-song-library.md#2-data-model)) | The candidate; `duplicate_of` recomputed |
| `PATCH /imports/{id}/candidates` | `{ decisions: [{ id, decision, merge_into?, merge_target_version?, remove_unmatched? }] }` (at most 500); `merge` needs `merge_into` and the `target_version` of its preview | The candidates; 409 `import_conflict` for an applied candidate |
| `GET /imports/{id}/candidates/{cid}/merge-preview` | query `merge_into` (song ID), `remove_unmatched` (default false) | `{ target_version, sections: [{ status: updated\|new\|kept\|removed, old_text?, new_text?, label }] }` — exactly what Apply would do now ([§3.2](#32-merge)) |
| `POST /imports/{id}/apply` | — | `{ created, merged, skipped, failed: [{ candidate_id, code }] }`; status `closed` when nothing is `pending` |
| `DELETE /imports/{id}` | — | 204 (batch and candidates deleted) |

`Candidate` = `id`, `draft`, `duplicate_of: { id, title, hymnal_source, hymnal_number }`, `decision`, `merge_into`, `merge_target_version`, `remove_unmatched`, `warnings`, `outcome`, `applied_song_id`, `error_code`. `duplicate_of`, `merge_into`, `merge_target_version`, `outcome`, `applied_song_id` and `error_code` are **left out** when they have no value (the API description language of Huma cannot mark a reference as nullable, as in [07 §4](07-readings.md#4-api)).

**Limits [P-51]:**

| Limit | Value | When exceeded |
|---|---|---|
| Body of `POST /imports` | 6 MiB (every other operation keeps the 1 MiB of [01 §8](01-foundation.md#8-http-basics)) | 413 `validation_failed` |
| Decoded text of all files together | 5 MiB | 413 `validation_failed` |
| One file or pasted text | 1 MiB of decoded text; pasted lyrics also at most 200,000 characters | That file is rejected, reason `file_too_large` (paste: 422 on the field) |
| Files per request | 200 | 422 `validation_failed` |
| Candidates per batch | 500 | 422 `validation_failed` |
| Complexity | XML: at most 100,000 tokens and depth 32; ChordPro: at most 20,000 lines per song; every parser checks `ctx` at least every 1,000 tokens or lines | That file or song is rejected, reason `too_complex` |
| One candidate's draft | 300 KB of JSON, 60 sections | `too_complex` / `too_many_sections` |

Parsers run in linear time and are cancelled with the request. Text is sent as a JSON string, so the rule of [03 §6](03-identity-auth.md#6-csrf-protection) is untouched; the browser reads files with `File.text()`.

Logging: info lines `import_created` (batch ID, format, number of candidates), `import_applied` (batch ID, counts). Lyrics are never logged.

## 6. Pages

| Route | Page |
|---|---|
| `/library/import` | Step 1: three choices — "Paste lyrics" (title, language, text box), "OpenLyrics files" and "ChordPro files" (file picker, several files allowed). Text says what the app accepts and that nothing is saved yet |
| `/library/import/{id}` | Step 2, review: a list of candidates (title, number of sections, warnings in words, duplicate notice with a link to the existing song); a preview of the selected candidate with its sections and an editor like [06 §4](06-song-library.md#4-pages); per candidate a choice **Add as new song**, **Merge into "…"** (offered with the duplicate preselected as a suggestion, never as a default; choosing it opens the **merge preview** of §3.2 with the checkbox "Remove the N sections that are not in the import", unticked), **Skip**; buttons "Add all without duplicates" and "Import chosen songs". After **Apply**: a summary and links to the new songs; failures are listed with their reason and a "Try again" button; a `target_changed` failure reopens the preview of the song as it is now |

Step 2 as built: **Merge** is offered only for a candidate with a detected duplicate and merges into that song (no search for another target); the editor covers title, other titles, language, hymnal, sections and order, and keeps the credit and licence fields as imported; warnings and the duplicate notice appear in the candidate list; the browser reads files as strict UTF-8 and holds back a file over 1 MiB, listing it with the reason (`not_utf8`, `file_too_large`) together with the files the server rejected; the review page also has "Cancel this import" (`DELETE /imports/{id}`, after a confirmation).

The library's empty state links here ([06 §4](06-song-library.md#4-pages)). Navigating away from a review keeps the batch for 7 days; the library page shows "You have an unfinished import" with a link while an `open` batch of the member's church exists.

## 7. Ports

| Port | Signature | Implementation | Used by |
|---|---|---|---|
| `Importer` | `Parse(ctx, r io.Reader, hint ImportHint) ([]ImportCandidate, error)` (exists since step 1, [04 §7](04-tenancy-extensions.md#7-extension-points)) | `adapters/importers/paste`, `…/openlyrics`, `…/chordpro`, registered by `server` in a map keyed by format; `server.WithImporter(format, Importer)` adds or replaces one | Import use case |
| `ChurchStore.Imports()` | batches and candidates: create, get, update draft, set decisions, mark applied, delete, delete older than | `adapters/sqlstore` | Import use case, cleanup job |

`app.ImportHint` and `app.ImportCandidate` stop being placeholders in this step (listed under **Ports** in `CHANGELOG.md`).

`Parse` returns one `ImportCandidate` per song found. A song that cannot be read is an entry with `Reject` set to the reason code (and no draft); the use case turns it into a `rejected` entry with its `song_index`. A document that cannot be read at all is an `*app.ImportUnreadableError{Reason}`.

`Cleanup.Hourly` visits every church in its own transaction to delete its old batches (`ImportRepo.DeleteOlderThan`), because repositories are church-scoped.

## 8. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Save imported songs before the review | Parse into candidates; save on **Apply** | SPEC §5.3: the user checks the result first |
| Write imported songs with direct SQL | Apply through the song use cases | Validation, church scoping and the search index must be the same as typing |
| Make one failing candidate roll back the whole batch | One transaction per candidate; report failures | One bad file shouldn't lose 49 good songs |
| Overwrite filled fields when merging | Fill empty fields only | The church's own edits are worth more than a re-import |
| Apply a merge that differs from what the preview showed, or reuse a section ID where the match is uncertain | Deterministic matching (§3.2), a version check on the target, and removal only when ticked | A merge can destroy lyrics the church edited, and later liturgies refer to section IDs |
| Resolve XML entities or DTDs, or fetch anything while parsing | Plain `encoding/xml`, no network | Hostile files |
| Trust the import's sizes, labels or numbers | The same validation as for typed songs, plus the limits of §5 | Imported data is untrusted input |
| Guess silently | Add a warning code for every guess | The user must see what was assumed |
| Log or return lyrics in logs and error messages | IDs and counts only | Size and copyright |

## 9. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-I-001 | Paste and split | The four worked examples of §4.1 | Exactly the sections, arrangements and warnings of the table | No repeats → no arrangement; `\r\n`; tabs and trailing spaces; leading blank lines |
| TC-I-002 | Labels | Every label of §4.1 in the three languages | Right kind and number; label removed from the text | `Bait 1:` with text on the same line; `Reff` alone later → arrangement entry only; `Chorus 2` → `number_dropped`; `第二节`; unknown label stays text |
| TC-I-003 | Verse numbering | `1.` `3.` unlabelled; duplicate `1.` | Numbers kept / renumbered with `verse_renumbered` | More than 60 blocks → `too_many_sections` |
| TC-I-004 | OpenLyrics | The OpenLyrics 0.8 sample song and a song with `verseOrder`, three author types, a songbook, markup in lines | Every mapping of §4.2 | Missing title → `no_song`; invalid key → `key_ignored`; unknown verse name; DOCTYPE with an entity is not expanded |
| TC-I-005 | ChordPro | A song with chords, `{sov}`/`{soc}`, `{subtitle}`, `{key}`; a file with two songs; directives from each row of the §4.3 table | Chords removed, sections and metadata mapped, two candidates; silent directives give no warning, `{comment}` gives `comment_ignored`, `{start_of_tab}` gives `block_ignored`, `{foo}` gives `directive_ignored` once | Lines outside blocks; `# …` comments; `{new_song}` first line; one malformed song among two |
| TC-I-006 | Duplicate detection | Candidates equal to an existing song by hymnal key, by title and language; two equal candidates in one batch | `duplicate_of` set; same title in another language → none; `duplicate_in_batch` on both | `Besar Setia-Mu` equals `besar setia mu` after folding (duplicate), but not `besar setiamu` (no duplicate) |
| TC-I-007 | Merge | Existing song with edited metadata and sections; drafts for each matching rule of §3.2 | Filled empty fields only; verses matched by number; a single chorus on each side matched; two choruses on each side matched only when their texts are identical, otherwise new; unmatched existing sections kept, removed only with `remove_unmatched`; arrangement points at the right IDs; version +1 | Section in use (stub) with `remove_unmatched` → `section_in_use`; target deleted → `not_found` |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-I-001 | Paste, review, apply | Editor | Batch created, nothing in the library; accept → song exists with the sections and is found by search | — |
| IT-I-002 | Partial failure and retry | Batch with one candidate made invalid after creation (e.g. title edited to 201 characters directly in the database) | Others applied, the failed candidate has `outcome = failed` with its code, the batch stays `open`; after the draft is fixed a second Apply applies it and closes the batch | — |
| IT-I-003 | Idempotent apply | Applied batch; a second batch | A second `apply` creates nothing; two simultaneous Applies of one batch (race harness, [02 §2.1](02-persistence.md#21-atomic-operations)) create each song once, on both dialects | — |
| IT-I-004 | Decisions and merge safety | Duplicates and non-duplicates | "Accept all" leaves duplicates `pending`; `merge` without `merge_into` or `merge_target_version` → 422, with another church's song → 404; the target edited after the decision → Apply fails with `import_conflict` / `target_changed` and nothing is changed; the preview matches the result of Apply exactly | — |
| IT-I-005 | Limits and permissions | Team member; member who lost `library.edit`; oversize body; 201 files; a 1 MiB + 1 file among valid ones; XML with 100,001 tokens | 403 on every route including `GET`; 413; 422; the large file rejected alone with `file_too_large`; the XML rejected with `too_complex`; a second member with the scope sees the first member's batch | — |
| IT-I-006 | Isolation | Two churches | Church B cannot read, change, apply or delete church A's batch (404) | — |
| IT-I-007 | Cleanup | Open batch older than 7 days, closed batch | Deleted by the cleanup job with their candidates; a recent open batch stays | — |
| IT-I-008 | Candidate edits versus Apply | An applied candidate; a race between `PATCH` and Apply (race harness) | `PATCH` of an applied candidate → 409 `import_conflict` (`already_applied`); in the race exactly one wins and Apply never works on a draft changed after it started, on both dialects | — |
| IT-I-009 | Multi-song file | ChordPro file with three songs, one malformed | Two candidates, one `rejected` entry with `song_index` | — |

Web tests: unit tests for the review page (decisions, duplicate suggestion never preselected, "Add all without duplicates"); end-to-end tests E2E-W-010 (paste lyrics, review, import, find the song) and E2E-W-011 (import an OpenLyrics and a ChordPro file); axe covers both import pages.

## 10. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| `import_unreadable` | 422, `reason` | `not_utf8`: "This file isn't plain text in UTF-8." `not_xml`: "This doesn't look like an OpenLyrics file." `no_song`: "No song was found." `too_many_sections`: "This song has more than 60 sections." `file_too_large`: "This file is too large (at most 1 MiB)." `too_complex`: "This file is too complicated to read." | Choose another file |
| Body too large | 413 `validation_failed` | "That is too much at once. Import fewer files." | Split the files |
| `validation_failed` on a draft | 422, field list | "Check this field." | Edit the draft |
| `import_conflict` | 409, `reason` | `already_applied`: "This song was already imported." `target_changed`: "The song to merge into was changed after you chose. Check the preview again." | Reopen the preview |
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
