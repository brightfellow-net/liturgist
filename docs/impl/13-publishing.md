# 13 — Publishing: Published Versions, Published View, My Assignments, Print, Reading Mode, WhatsApp Messages (Implementation)

> **Document type: Implementation.** Step 5 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), in five slices: 5A publishing, 5B reading views, 5C print, 5D reading mode and offline, 5E WhatsApp messages.
> Status: decisions **[P-xx] Approved** 2026-10-05 (after the Spec Gate, 9.0/10 self-scored, and adversarial review round 5). Code may be written from this document. Items marked **[P-xx]** are decisions listed in the [index](README.md#4f-decisions-step-5). The changes it makes to earlier documents ([§11.1](#111-changes-to-earlier-documents-applied-in-the-docs-commit-that-follows-the-owners-approval)) were applied on approval.

## 1. Scope

Everything SPEC §5.5 and §5.6 need for the GKY Citragarden pilot ([PILOT.md](../PILOT.md)): `approved` → `published` with a stored copy (`PublishedVersion`), reopening and archiving a published liturgy, the published view for every member, "my assignments", the print view, reading mode with offline use, and the WhatsApp texts. Scope chosen by the owner on 2026-10-05: all of it, in slices; print by stylesheet only; no PDF layout reference yet (a generic layout, restyled later).

| Left out | Why | Where it lands |
|---|---|---|
| Server-side PDF (Typst), A5 booklets, exact match with the GKY document | Owner decision 2026-10-05: print stylesheet only; open decision 3 (the GKY document) is not closed | After the pilot starts |
| Congregation version, "my part" print variant | SPEC §5.6 lists them after the MVP | Later |
| Guest links without login | SPEC §11 (2026-10-02): every link needs login | If the pilot asks |
| Viewing or restoring an **older** `PublishedVersion` | Versions are stored (SPEC §11) but no page shows them; the change summary (§8) is the only comparison | After pilot feedback |
| Church logo in the print header (upload through `Storage`) | Owner decision 2026-10-05 (Q-5.1): the header shows the church name | After the first pilot week |
| Editable message templates, sending WhatsApp or e-mail automatically, reminders | SPEC §5.6 | Later |
| Slides, OpenLP, PPTX | SPEC §10 | After the pilot |
| A history row for archive and unarchive | They are not state changes; they are logged (§4) | If the pilot asks |

## 2. Publishing [P-75]

The fifth transition of [12 §2](12-review.md#2-states-and-transitions-p-68), built the same way.

| Action | From → to | Scope | Body | Response |
|---|---|---|---|---|
| `publish` | `approved` → `published` | `liturgy.approve` (SPEC §4) | `{ edit_seq, note? }` | 200 the liturgy view (with `published`, below) |

- **Same mechanics as the other transitions:** one conditional update on `state`, `edit_seq` **and `archived_at IS NULL`** (a change to `Transition`, applied to every action; only a published liturgy can be archived, so for the others it is a no-op kept for uniformity), a `liturgy_state_changes` row, `undo_floor_seq` set, the same order of checks (404, 403, 422, 409 `invalid_transition`, then the update). `edit_seq` is required, as for approve ([12 P-70](12-review.md#2-states-and-transitions-p-68)): an approved liturgy can be reopened, edited and approved again, and a tab left open must not publish content its user never read. A mismatch is 409 `review_stale`.
- **When the update changes no row, the re-read decides in this order [P-75]:** liturgy gone → 404; state differs from the one read → 409 `invalid_transition`; `archived_at` set → 409 `liturgy_archived`; otherwise → 409 `review_stale`. (This extends the re-read of [12 §2](12-review.md#2-states-and-transitions-p-68) by the archived branch, for `Review`'s `reopen`.)
- **The copy is made in the same transaction.** After the update has locked the liturgy row, the transaction (a) re-computes `problems` and, if any exist, rolls back with 422 `has_problems` (defence in depth: step 4 already refuses them at submit and an approved liturgy cannot change, so this cannot happen through the API and is tested with a hook), (b) builds the content of §3, (c) takes `number = COALESCE(MAX(number), 0) + 1` of this liturgy (safe: the row lock is held, and `UNIQUE (church_id, liturgy_id, number)` backs it), (d) inserts the `published_versions` row and the `published_assignees` rows (§9). If any step fails, the state change rolls back with it: **a liturgy is never `published` without a version.** No external call is made inside the transaction. The SQLite write lock and the PostgreSQL row lock are held for the build (a few reads and two inserts); that is accepted.
- **Each song is read with its sections in one statement** (one join), so a lyrics edit by someone else, which does not lock the liturgy, is either wholly in or wholly out of that song's copy. A song edited between two songs of the same liturgy gives a copy that is consistent per song; that is accepted.
- **Size [P-75].** The cap applies to the **bytes that are stored**: the content is serialized with `json.Encoder.SetEscapeHTML(false)` and the length of that output is compared with **4 MiB (4,194,304 bytes)**. Over the cap → 422 `publish_too_large` with `largest_item` (the title of the item with the longest serialized form), nothing changes. The limits of [10 §2](10-liturgy.md#2-data-model) do **not** guarantee that the cap is never reached (60 items of 20,000 Chinese characters are about 3.6 MB); the cap protects the database and the response size, and a realistic liturgy (60 items, 10 songs of 12 sections each) is far below it. Sections are stored once per item song, so a repeated chorus costs nothing.
- **Republishing.** A published liturgy is reopened (§4), edited, submitted, approved and published again: `number` + 1. Publishing is the only way a version is made.
- **Deleting.** A liturgy with **any** version cannot be deleted, in any state (409 `liturgy_not_deletable`); this is a change to [10 §2.1](10-liturgy.md#21-liturgy) and to SPEC §8.2.1 rule 3 (logged in the SPEC decisions log). `Liturgies().Delete` becomes `DELETE FROM liturgies WHERE church_id = ? AND id = ? AND state <> 'published' AND NOT EXISTS (SELECT 1 FROM published_versions WHERE church_id = ? AND liturgy_id = ?)`; zero rows → re-read: gone → 404, else 409 `liturgy_not_deletable`. The foreign key from `published_versions` to `liturgies` is `NO ACTION` (§9), so a delete that still gets through (a publish that commits between the statement's check and its write) fails the foreign key and the repository maps it to the same 409. The way to free a slot held by a reopened liturgy that was once published is to republish it and archive it. Without this rule, a delete racing a publish, or a delete of a reopened liturgy, would destroy the copy the team rehearses from and the licence record (SPEC §11).
- **Limits.** Publishing frees an unpublished slot and uses none, so it is never limited ([SPEC §8.2.1](../SPEC.md#821-usage-limits-saas-free-plan)).
- **`actions`** of the liturgy view gain `publish` (scope and state `approved`), `archive`, `unarchive` (§4); `reopen` is also true in `published` for a non-archived liturgy. The view gains `published: { number, published_at } | null` for members with a `liturgy.*` scope (the latest version).
- **Logging:** `liturgy_published` (IDs, `number`, content size, actor), `liturgy_publish_too_large`, `liturgy_reopened` (with the state it left), `liturgy_archived`, `liturgy_unarchived`; never content.

## 3. PublishedVersion [P-76]

Table `published_versions` ([schema](../reference/schema.md#step-5-tables)): `id`, `church_id`, `liturgy_id`, `number`, `content` (text holding JSON), `published_by`, `published_at`. Immutable: no update route, no delete route; rows disappear only when the church does. The `content` is **format 1**, a self-contained copy: rendering it needs no other table (a deleted song, a renamed duty or a changed church setting cannot alter what was published).

| Part | Content |
|---|---|
| `format` | `1`. The app keeps reading format 1 for as long as versions of that format exist; a reader that meets a larger number shows "update the app" instead of guessing |
| `liturgy` | `date`, `time`, `service_name`, `language`, `church_name` |
| `items[]` | `id` (the item's ID, used to compare versions and to find "my part"), `position`, `type`, `title`, `duty` (`{ id, name }` — **the displayed name at publishing**, owner requirement Q-3.9 — or null), `text` (types with text) |
| `items[].reading` | `reference_display`, `translation_code`, `text`, `attribution`; or null for a non-reading item |
| `items[].songs[]` | `song_id`, `title`, `hymnal_source`, `hymnal_number`, `key` (letters, not the display form), `note`, `copyright_holder`, `copyright_line`, `ccli_song_number`, `sections[]` (`id`, `kind`, `number`, `label`, `text`; only the sections the sequence uses, each once), `entries[]` (`section_id`, `part` = `{ id, name }` or null, `key_change`, `note`) |
| `assignments[]` | `duty` (`{ id, name }`), `user_id` (or null), `name` (the person's name **as shown at publishing**: `users.name`, or the free text) |
| `licence_footer` | The church's `licence_footer` setting at publishing (§6), possibly empty |

- **Section labels are fixed at publishing.** `label` is the section's own label, or, when it has none, the label derived from `kind` and `number` **in the liturgy's `language`** (not the viewer's UI language, [06 §2.2](06-song-library.md#22-section)), so the same copy reads the same for every viewer and on paper.
- **Not stored:** the display form of keys (`Do = G`) and the show/hide choices of credits and print options: they are applied when rendering, from the current church settings. The **text** of a credit line or licence footer is the one at publishing (SPEC §7), but `show_credits` is live: a church that turns credits off sees them disappear from old versions too.
- **Never rendered as HTML:** every text is escaped by the client; lyrics, readings and notes are plain text with `\n`.
- **Code:** `domain/published.go` (the content types and `Compare`, §8), `app/liturgy_publish.go` (the builder reads through the existing `ChurchStore` repositories), `adapters/sqlstore/published.go` (`PublishedRepo`: `Create`, `Latest`, `Previous`, `Exists`, `ListLatest`, `Upcoming`).

## 4. Reopening, archiving, unarchiving [P-77, P-78]

| Action | From → to | Scope | Extra rules |
|---|---|---|---|
| `reopen` (extended) | `published` → `draft` (and `approved` → `draft`, [12 §2](12-review.md#2-states-and-transitions-p-68)) | `liturgy.approve` | Not archived (409 `liturgy_archived`). **Every reopen loads the actor with `LockChurch` first** (`actorIn(lock=true)`), then reads the state; only when the locked read says `published` does it call `Entitlements.Limit(max_unpublished_liturgies)`: `used + 1 > max` → 403 `limit_reached` ([10 §8](10-liturgy.md#8-entitlements-and-limits)). The lock is held for the few statements of the reopen and is never taken together with another liturgy row |
| `archive` | `published` → `published` with `archived_at`, `archived_by` set | `liturgy.manage` | `UPDATE … WHERE state = 'published' AND archived_at IS NULL`. Not a state change: no history row. Never limited, and never takes `LockChurch` (it only lowers the counts). Re-read when no row changes: gone → 404; state not `published` (including a reopened liturgy) → 409 `invalid_transition`; already archived → 409 `liturgy_archived` |
| `unarchive` | clears both columns | `liturgy.manage` | `UPDATE … WHERE archived_at IS NOT NULL`. Takes `LockChurch`, then `Limit(max_active_liturgies)`: `used + 1 > max` → 403 `limit_reached`. Not archived → 409 `not_archived` |

- **Reopening a published liturgy [P-77].** The liturgy goes to `draft`; its last `PublishedVersion` stays and **members keep seeing it with a "being revised" notice** until the next publish (SPEC §11, 2026-10-02). Editing, submitting, approving work as in step 4. **References cleared by a library deletion (P-61) do not block the reopen [owner decision 2026-10-05]:** the items show as `problems` (`song_removed`, `reading_removed`, `section_removed`) with their title snapshots, the editor replaces them ([10 §6](10-liturgy.md#6-usage-ports-and-deleted-songs-readings-sections)), and the next submit is refused until they are fixed ([12 P-69](12-review.md#2-states-and-transitions-p-68)). The published version is not touched.
- **The limit on reopen is mandatory** (10 §8) and has a cost worth naming: a church already at `max_unpublished_liturgies` cannot reopen a published liturgy to correct a typo until it publishes or deletes another unpublished one. Archiving does **not** free an unpublished slot, so the message differs per limit: unpublished → "Finish or delete an unpublished liturgy first."; active (unarchive) → "Archive another liturgy first." The community edition is unlimited.
- **Archived liturgies** stay readable in the published view and print on every plan (SPEC §11); they are locked (state `published`), cannot be reopened, deleted or submitted, and do not count as active. They appear in the editor list as before and in the published list only with `archived=true` or `all`.
- **Archive and a concurrent reopen:** both are conditional updates on the same row; exactly one wins and the other re-reads and answers as above.
- **"Archive last week's N published liturgies and create these N" [P-78]** extends `POST /liturgies/prepare` ([10 §3.1](10-liturgy.md#31-prepare-next-week-p-59)) with an optional `archive_ids` (1–50 liturgy IDs; all-or-nothing, as the rest of the request):
  - Needs `liturgy.edit` (as prepare) **and** `liturgy.manage`; without `liturgy.manage` a request with `archive_ids` → 403, checked before anything else is read.
  - Every ID must be a non-archived `published` liturgy of this church whose `date` is **before the Monday of the week the occurrences belong to** (the earliest occurrence's week). Anything else → 409 `invalid_transition` naming the first ID, nothing is changed. An unknown ID → 404.
  - Order inside one transaction under `LockChurch`: check the scopes → validate the IDs → archive them → count the limits → create. If the limits still stop the creation after the archiving (for example because `max_unpublished_liturgies` is the one that blocks, which archiving cannot free) → 403 `limit_reached` and **the transaction rolls back, so nothing is archived**. The web app offers the button only when archiving would be enough, but the server never relies on that.
  - The response is the prepare response plus `archived: [id]`. Logged as `liturgy_archived` per liturgy.

## 5. Reading the published copy [P-79, P-80]

All routes are under the church path and need a session. **A member holds the right to read a liturgy's published copy when the church has at least one version of it**, whatever their scopes and whatever the liturgy's current state or archive flag. This is a new resource with its own check; it never uses `openLiturgy`. Not a member of the church, or no version: 404.

**Required change to step 3 [P-79].** Today `openLiturgy` and the scope-less fallback of `Liturgies.List` let every member read a `published` liturgy ([10 §2.1](10-liturgy.md#21-liturgy), `app/liturgy.go`), which would give a team member the **editable** view, `edit_seq`, `actions` and the edit history (authors, before/after images) of every published liturgy, and make the same URL answer 200 or 404 depending on the state. Step 5 removes the `StatePublished` exception from both: a member without a `liturgy.*` scope gets **404 (or an empty list) from `GET /liturgies`, `/liturgies/{id}`, `/liturgies/{id}/edits`, `/liturgies/{id}/state-changes`, `/liturgies/{id}/comments` and `/liturgies/{id}/assignments` in every state**; the published routes below are their only way in. 10 §2.1 and the web `/liturgies` list are updated accordingly (12 P-74 already says this for comments and state changes).

| Method & path | Response |
|---|---|
| `GET /published?from=&to=&archived=&limit=&offset=` | `{ items: [{ id, date, time, service_name, language, number, published_at, revising, archived }], total }`. `id` is the **liturgy** ID. `from`, `to`: inclusive `YYYY-MM-DD` (422 on any other shape). `archived`: `false` (default), `true` or `all` (422 otherwise). Ordered by date, then time (empty last), then ID, descending. `limit` default 50, at most 100 (a larger value → 422). `revising` = the liturgy's state is not `published` |
| `GET /liturgies/{id}/published` | `{ number, published_at, published_by: { id, name }, revising, archived, content, render, url }`. `render` = the **current** church settings for display (`key_display`, `show_credits`, `print` defaults, §6); `url` = `URLBuilder.AppURL("/published/{id}")` |
| `GET /me/assignments` | `{ items: [{ liturgy: { id, date, time, service_name, language, number, revising }, duties: [{ id, name }], items: [{ id, title, type }] }], more }`, soonest first |

- **Caching:** the three routes answer `Cache-Control: private, no-store` and carry no `ETag`: archiving, reopening and print settings change the response without changing `number`, so a validator on `number` would serve stale banners. (The offline cache of §7 is the service worker's own.)
- **"My assignments" [P-80]** are the duties of the **logged-in user** in the **latest version** of each non-archived liturgy dated today or later, where "today" is the church's current calendar date in its time zone and the comparison is on the `date` string only (a 07:00 service is still upcoming at 18:00). The match uses the table `published_assignees` (§9: the user IDs of a version, written with it), so no content is read for liturgies that do not match: `Upcoming(userID, today, limit 50)` selects, per liturgy, the version with `MAX(number)`, joins `published_assignees` on that version and the user, orders by date and time, and returns at most 50 (`more` is true when a 51st exists). For each match the use case reads the version's `content` to fill `duties` (the entries with this `user_id`) and `items` (items whose `duty.id` is one of those duties). A liturgy being revised shows the **published** assignments: what is in the editor is not shown until it is published. A person's removal from the church changes nothing in a version.
- **Phone numbers and e-mail** appear in none of these three responses (a test checks it, IT-P-010).

## 6. Print view [P-81]

A page of the SPA, `/published/{id}/print`, rendered **from the stored copy** with `@page` and print CSS; "Print / Save as PDF" calls `window.print()`. No server-side PDF in step 5.

| Option | Values | Default source |
|---|---|---|
| `variant` | `team` (every item, who leads, lyrics by sequence with singing parts, readings, keys, notes, assignments), `musician` (songs only: hymnal number, key and key changes, sequence, parts, notes, first lines; no credits) | `team` |
| `paper` | `a4`, `f4` (215 × 330 mm) | church setting |
| `lyrics` | `full`, `first_lines` (the first line of each section's text) | church setting |
| `readings`, `assignments`, `keys`, `notes` | on / off | church setting |
| `size` | `normal`, `large` | church setting |

- **Church settings [P-81]** are new keys of `churches.settings` (no migration; unknown keys are preserved, [schema](../reference/schema.md#churches)): `show_credits` (bool, default true), `licence_footer` (0–200 characters, e.g. "CCLI License #1234567"; the empty string clears it), `print` = `{ paper, lyrics, readings, assignments, keys, notes, size }` with the defaults `a4`, `full`, true, true, true, true, `normal`.
  - **Required code change.** `domain.ChurchSettings` (today three string keys and `Extra`) gains `ShowCredits *bool`, `LicenceFooter string` and `Print *PrintDefaults`; `knownSettings`, `MarshalJSON` and the parse follow; unknown keys are still kept.
  - **`PATCH /church` ([04 §6](04-tenancy-extensions.md#6-step-1-church-and-member-api))** gains `show_credits`, `licence_footer` (the empty string is allowed, unlike the other text fields) and `print` (a **complete** object that replaces the old one; every sub-key required; a missing sub-key or any value outside the lists → 422 `validation_failed`). The update takes `LockChurch`, so two simultaneous PATCHes apply one after the other and each keeps the other's keys. 04 §6 and the OpenAPI document are updated with it.
  - The Church settings page gains a "Printing" section.
- **The person printing can change every option** on the print page (kept in the page's query string, so a link keeps the choice; no personal data in it; an invalid value falls back to the church default). Choosing never changes the church defaults.
- **Header:** church name, service name, date and time (church time zone). Credits (`copyright_line` under each song, never on the musician sheet) and the licence footer follow `show_credits`; the footer is printed when non-empty.
- **Layout rules (best effort, tested by CSS rules, not by page counts):** a song heading stays with its first section, sections are not split across pages where possible, a row of the sequence is not split. The musician sheet is compact so that a normal liturgy fits one page, but this is not enforced. Browser headers and footers (URL, page numbers) belong to the browser and cannot be controlled.
- **Archived and being revised** liturgies print their last version; a "being revised" liturgy shows no marker on paper.

## 7. Reading mode and offline use [P-82]

Reading mode is a toggle on the published view (`?read=1`), for reading or leading from a phone during the service.

| Part | Rule |
|---|---|
| Layout | One column, large text, high contrast, no menus; sized in relative units; works at 320 px width and 200% text size. The `text_size` preference ([04 §6](04-tenancy-extensions.md#6-step-1-church-and-member-api)) applies; the published view and "my assignments" carry the "A · A+ · A++" control that saves it (`PATCH /me`) |
| My part | Items whose `duty.id` is one of the viewer's duties in `assignments[]` are highlighted (in text, not by colour alone); a "Go to my part" button scrolls to the first one and moves focus to it; hidden when the viewer has none |
| Screen on | A "Keep screen on" switch using the Wake Lock API; hidden when the browser lacks it; released when the page is hidden or the switch is turned off; re-requested when the page becomes visible again while the switch is on |
| Caches | Two Workbox caches, both **per app build** (old builds' caches are deleted when the new service worker activates): `shell` (the app's static files, precached) and `pub` (`NetworkFirst`, network timeout 4 s, `maxEntries: 5`, only `GET <church path>/liturgies/{id}/published` and the fixed URL `GET <church path>/me/assignments`; the lists, `/me`, `/church`, `/published?…` and every other route are **never** cached). Route patterns are built from the same church path prefix as `URLBuilder.AppPath` (SaaS paths) |
| Offline notice | Workbox serves a cached answer as a normal 200, so the page cannot see a failure. A small plugin adds the header `X-From-Cache: 1` to every response it takes from the cache, and the client shows "Offline — showing the last saved copy" with the copy's `published_at` when it sees that header. With a slow connection (the 4 s timeout) the cached copy is shown with the same notice. Offline pages are read-only; writes fail with the normal error and are never queued |
| Opening offline | The app shell boots offline; the sign-in state is not re-checked (`GET /me` is not cached), so the shell shows the cached published views and assignments directly when the network is down and a **user marker** (the user ID, in `localStorage`) exists; otherwise the login page |
| Clearing | (As built, the `shell` cache is kept; see [§11.5](#115-slice-5d-as-built-2026-10-05).) All caches and the user marker are deleted: at logout (the client deletes them **first**, before and whatever the answer of the API call, so logging out offline clears too), on any 401, when a login finds a different user ID than the marker, and when the church path changes. A fetch that finishes after a clear must not repopulate the cache: the plugin compares a generation number that every clear increments. A copy otherwise stays until logout or 401: a member removed from the church keeps their cached copies until then (accepted; nothing new is fetched) |
| Privacy | The cache holds published views of one user's phone. The privacy notice ([05 §2](05-web-shell.md#2-pages-in-step-1), `/privacy`) is updated to say so, and to say which members can see a phone number (§8) |
| Install | A web app manifest (name, icons, `display: standalone`, theme colours) so the app can be added to the home screen; the welcome text of SPEC §5.7 explains it |

## 8. WhatsApp messages and change summary [P-83]

After publishing, the page offers three texts (SPEC §5.6). The server sends **structured data**; the web app builds the text, in the **liturgy's language** (`i18next` with `lng` = the version's `language`, falling back to the church's default UI language when there is no translation, e.g. `zh-*` before Chinese strings exist).

| Route | Scope | Response |
|---|---|---|
| `GET /liturgies/{id}/published/summary` | `liturgy.approve` | `{ number, url, items: [{ title, type, duty: { id, name } \| null, songs: [{ title, hymnal_source, hymnal_number, key }], reading: { reference_display, translation_code } \| null }], assignments: [{ duty: { id, name }, names[] }], recipients: [{ name, duties[], phone? }], changes? }` |

As built, the response also has `liturgy`, `key_display` and `member` on each recipient; see [§11.6](#116-slice-5e-as-built-2026-10-05).

- **Who.** `liturgy.approve`, the scope that publishes (the default Liturgist and Church admin roles hold it); the reason to keep it narrow is that the response carries phone numbers. An editor who only holds `liturgy.edit` cannot send the texts. The response is `Cache-Control: private, no-store`, and the service worker never caches it (§7).
- **Recipients [Q-5.2].** One entry per assigned **member** of the latest version who is still a member of the church. `phone` is the member's current number in E.164 (`users.phone`), present only here; free-text assignees are listed with no phone; members with no phone get a Copy button, not a WhatsApp button. The client builds `https://wa.me/<digits>?text=<encoded>`; the team text uses `https://wa.me/?text=`. The sender presses send in WhatsApp; the app sends nothing. The privacy notice says that members who can approve liturgies see the numbers of the people assigned.
- **Content rules.** Titles, hymnal numbers, keys, reading references, assignments and the link, **never lyrics or Bible text**; checkboxes decide whether songs, keys and readings are included; WhatsApp formatting only (`*bold*`, plain URLs); short lines; the personal message greets the person by name and avoids "kamu" and "Bapak/Ibu". The key is shown in the church's `key_display` form.
- **Length.** A text has at most **1,500 characters** (counted in runes, "…" included); this is the app's own limit for a message that stays easy to read on a phone, not a WhatsApp rule. Fixed blocks, never dropped: the heading (service, date and time), the person's own lines (personal text) or the assignments block (team text), and the link line. The only droppable block is the **item list**; when the text is too long, whole item lines are removed from the bottom and one line "…" is added. If the fixed blocks alone exceed the limit, the text is shown whole with a warning "This message is long".
- **Change summary.** `changes` is present when a previous version exists: `Compare(previous, latest)` in `domain/published.go`, matching **items by `id`**, **songs by `song_id`** within an item and **assignments by (duty `id`, person key)**, where the person key is `user_id` when set, otherwise the free text trimmed and case-folded. It reports: items added and removed; items **moved** (see below); an item's title or `duty` changed; songs added, removed, key changed (including `key_change` of entries); the reading's reference changed; people added or removed per duty. **Text edits (lyrics, readings' words, free text) are not reported** (they cannot be sent).
  - **Moved** means a change of the item's **relative order** among the items present in both versions: the common items are taken in each version's order and the longest common subsequence stays, every other common item is reported as moved. Shifts caused only by items added or removed are not reported (inserting one item at the top reports one addition and no moves).
  - `changes` is `{ items: [...], songs: [...], reading: [...], assignments: [...] }`, empty arrays when nothing changed, and the page says "Nothing the team needs to know changed."
- **Preview.** The page shows each text before the buttons; Copy uses the Clipboard API with a selectable text fallback.
- **Link.** `url` comes from `URLBuilder.AppURL`; when `LITURGIST_BASE_URL` is not set the page shows a warning before the link is copied (the startup warning of step 1 exists).

## 9. Data model [P-84]

One migration for step 5, `00010_published.sql`, in both dialect folders. Tables in [reference/schema.md](../reference/schema.md#step-5-tables).

| Table | Purpose |
|---|---|
| `published_versions` | One row per publication of a liturgy (§3). Its foreign key to `liturgies` is `NO ACTION`, so a liturgy that has a version cannot be deleted (§2) |
| `published_assignees` | The user IDs of the assignments of one version, written in the same transaction; immutable; serves "my assignments" without reading content (§5). It repeats `assignments[].user_id` of the same immutable row, so it is an index, not a second source that can drift |

`liturgies` keeps its columns: `state` (`published`) and `archived_at`, `archived_by` exist. The latest version is `MAX(number)` of the liturgy; there is no copy of it on the liturgy row. Church settings are JSON keys (§6), no migration.

## 10. Pages

All text through i18n ([05 §6](05-web-shell.md#6-translations)); the pages work at 320 px width and 200% text; no modal dialogs.

| Place | What | Slice |
|---|---|---|
| Liturgy page review bar | "Publish" (with the optional note), "Archive", "Unarchive", and "Reopen" in `published`; the "being revised" and "published" banners; the number and date of the last version; the per-limit messages of §4 | 5A |
| `/liturgies` list | A "published" filter chip already works through `state`; archived liturgies are hidden unless "Show archived" is ticked | 5A |
| Prepare next week | The archive-and-create button of §4 when archiving the previous published liturgies is enough to make room | 5A |
| Navigation | Reconciles [05 §2](05-web-shell.md#2-pages-in-step-1) with SPEC §5.7: **every member** sees **My assignments** (Tugas saya), **Published** (Liturgi, the published list), **Library** (kept, a step 2 decision) and **Profile**; members with a `liturgy.*` scope also see the planning menu (**Liturgies**, the editor list, as now); **Settings** as now. A member with no scope lands on My assignments after login and sees the welcome text of SPEC §5.7 (05's "Home" placeholder is replaced). 05 §2 is updated | 5B |
| `/published` | The published list, newest first, a "Show archived" switch, a "Being revised" tag | 5B |
| `/published/{id}` | The published view (header, items in order, songs with sequence and singing parts, readings with attribution, assignments, credits), the notice when revising or archived, the text-size control, links to Print and Reading mode | 5B |
| `/me/assignments` | The cards of §5, each linking to the view and to the first item of the viewer | 5B |
| `/published/{id}/print` | The options bar and the page to print | 5C |
| Church settings → Printing | `show_credits`, `licence_footer`, `print` defaults | 5C |
| Reading mode, offline notice, manifest | §7 | 5D |
| After publishing, and on the published view for `liturgy.approve` | "Send to the team": the three texts, checkboxes, Copy and WhatsApp buttons, the recipients list | 5E |

Keyboard and screen readers: buttons have text; the published view is a real heading structure (liturgy → item → song); "Go to my part" moves focus; the offline notice is a live region; the print page options are a labelled group.

## 11. Slices

| Slice | Content |
|---|---|
| 5A | Migration `00010_published.sql`, `publish`, the snapshot builder, `published` in the view, the conditional `Delete`, `reopen` from `published` with the locked limit check, `archive` / `unarchive`, the archived guard in `Transition`, the archive-and-create extension, review bar changes, the removal of the `published` exception from `openLiturgy` and `List` (with 10 §2.1) |
| 5B | The three read routes, navigation for every member, the published list and view, "My assignments", the text-size control |
| 5C | Church print settings (domain, `PATCH /church`, page), the print view, both variants |
| 5D | Reading mode, Wake Lock, service worker and cache rules, manifest, privacy notice text |
| 5E | The summary route, `Compare`, the messages page |

Each slice is drafted, shown and approved on its own, as in steps 3 and 4. 5D can be cut or postponed without touching the others.

### 11.1 Changes to earlier documents (applied in the docs commit that follows the owner's approval)

Applied 2026-10-05 in the commit that approved this document.

| Document | Change |
|---|---|
| [10 §2.1](10-liturgy.md#21-liturgy) | "Published liturgies are visible to every member" becomes: members without a `liturgy.*` scope read published liturgies only through the routes of §5; **Deletable**: never when any version exists (409 `liturgy_not_deletable`) |
| [10 §3.1](10-liturgy.md#31-prepare-next-week-p-59) | `archive_ids` on `POST /liturgies/prepare` (§4) |
| [10 §8](10-liturgy.md#8-entitlements-and-limits) | "in step 4" becomes "in step 5"; the per-limit messages of §4 |
| [04 §6](04-tenancy-extensions.md#6-step-1-church-and-member-api) | `PATCH /church` gains `show_credits`, `licence_footer`, `print` and the empty-string exception (§6) |
| [05 §2](05-web-shell.md#2-pages-in-step-1) | Navigation and Home as in §10; the privacy notice text of §7 and §8 |
| [12 §2](12-review.md#2-states-and-transitions-p-68) | `reopen` from `published`; the re-read order with the archived branch; `Transition` also requires `archived_at IS NULL` |
| [SPEC.md §8.2.1](../SPEC.md#821-usage-limits-saas-free-plan) rule 3 and §11 | A liturgy that has been published cannot be deleted even after a reopen (decision-log row) |

### 11.2 Slice 5A as built (2026-10-05)

Merged as `bb3d6cc`. Differences and additions to the text above:

- **Migration** `00010_published.sql` (both dialects): `published_versions` (foreign key to `liturgies` `NO ACTION`) and `published_assignees`. `published_assignees` is written now and read from slice 5B on.
- **Code.** `domain/published.go` (the content types, `SectionLabelIn`), `domain.ActionPublish` and `reopen` from `published` in the transition table, `app/liturgy_publish.go` (`buildPublished`, `Archive`, `Unarchive`), `PublishedRepo` (`Create`, `Info`, `Exists`; `Latest`, `Previous`, `Upcoming` come with 5B and 5E), `LiturgyRepo.Archive` / `Unarchive`, `Transition` requires `archived_at IS NULL`, `Delete` is conditional, `List` takes an `Archived` filter and reports `HasVersions`, `Liturgies.PrepareCreateArchiving`.
- **Song reads.** The song row and its sections are two statements, not one join; the sections (the text) are one statement, so the text of a song is consistent as a set.
- **Licence footer.** `licence_footer` is stored empty until slice 5C adds the church setting.
- **Archived liturgies in the list.** `GET /liturgies` hides them unless `archived=true` or `all` is given (§10 wins over the "as before" of §4); the page has a "Show archived" switch.
- **Archive-and-create.** The page archives the oldest published liturgies dated before the week's Monday (the doc said "last week's"); the server checks every ID. `archive_ids` and the response field `archived` are on `POST /liturgies/prepare`.
- **Logs.** `liturgy_published`, `liturgy_reopened`, `liturgy_archived`, `liturgy_unarchived` carry the actor and the liturgy ID; the state left and the content size are not logged.
- **Messages.** The per-limit texts replace the old liturgy-limit message on every path, create included. The Indonesian texts and the Indonesian section words (Bait, Pra-refren, Refren, Jembatan, Tag, Intro, Penutup, Bagian) are drafts for the owner.
- **Tests.** TC-P-001; IT-P-001, 002, 003, 004 (free-running delete race and the foreign-key backstop, not forced interleavings for reopen and library delete), 005, 006 (including the forced PostgreSQL interleaving `TestReopenLimitForced`), 007, 008, 011, 013, 015; `TestReviewVisibility` rewritten for P-79; WT-P-001 and the prepare button tests; E2E-W-017. Removing the church lock on reopen, the conditional delete (the foreign key still refuses), the `published` exception in `openLiturgy`, or the version insert each fails a test. Go on both dialects; Vitest 225; Playwright 37. Not covered by their own test: the `archived_at IS NULL` condition of `Transition` (the use case checks first). `SelectForTest` is a new test hook; the scoped-repository snapshot also covers comments and state changes now.
- **Not run:** golangci-lint (the v1/v2 config mismatch of step 3 still stands).

### 11.3 Slice 5B as built (2026-10-05)

Merged as `ec7f4c4`. Differences and additions to the text above:

- **Routes.** `GET /published` (`from`, `to`, `archived`, `limit` 0 to 100, `offset`), `GET /liturgies/{id}/published` and `GET /me/assignments`, all `TenancyChurch`, tag `published`, `Cache-Control: private, no-store`, no ETag. They never return phone numbers or e-mail addresses.
- **Code.** `app/published_read.go` (`ReadPublished`, `ListPublished`, `MyAssignments`), `PublishedRepo` gains `Latest`, `ListLatest` and `Upcoming` (`Upcoming` reads the newest version with `MAX(number)`, then calls `Latest` for at most 51 rows). `Liturgies` gains `URLs`. `ReadPublished` does not go through `openLiturgy`.
- **Home page.** "My assignments" is the home page `/` under the Welcome heading, not a separate page; every member lands there. The nav is My assignments, Published, Library, Profile, then Liturgies and Settings as before. The old "Home" label is removed.
- **Indonesian nav label.** "Liturgi terbit" for Published (the doc's "Liturgi" is already the planning menu's word). All new Indonesian strings, including the home-screen steps, are drafts for the owner.
- **Absent, not null.** Huma cannot mark a struct pointer nullable, so `duty`, `reading` and `part` are left out when empty.
- **`render`.** Only `key_display` and `show_credits` (always `true`) until slice 5C adds the setting and the `print` defaults.
- **Header source.** The list and the cards take date, time and name from the current liturgy row, not from the stored copy, so a reopened liturgy with a changed date shows the new date over the old content, with the "Being revised" tag.
- **Left out until later.** The "Print" link (5C), the "Reading mode" link (5D) and the card link to the user's first item.
- **Text size.** The control sends `{text_size, ui_language}` to `PATCH /me`, without the name.
- **Tests.** `TestReadPublished`, `TestListPublished`, `TestMyAssignments` (church time zone, a person removed in version 2), `TestMyAssignmentsMore` (cut at 50), `TestReadPublishedHTTP`, 12 Vitest tests (WT-P-002, WT-P-003), E2E-W-017 extended. Dropping the newest-version condition of `Upcoming`, or reading the copy through `openLiturgy`, each fails a test. Go on both dialects; Vitest 237; Playwright 37 (the whole suite once, then E2E-W-017 again after a locator fix).
- **Not run:** golangci-lint (the v1/v2 config mismatch of step 3 still stands).

### 11.4 Slice 5C as built (2026-10-05)

Merged as `d7cae7c`. Differences and additions to the text above:

- **Settings.** `domain.ChurchSettings` gains `ShowCredits *bool` (not set reads as true), `LicenceFooter` and `Print *PrintDefaults`; a known key of the wrong type, or a `print` object with a value outside the lists, reads as not set. `ValidateChurch` trims the footer and checks 200 characters and the `print` values. `PATCH /church` and `GET /church` carry the three keys; `GET` always shows the defaults filled in. 04 §6 already lists them.
- **Footer.** `buildPublished` copies the church's footer into the copy at publish. A changed footer reaches old copies only at their next version; `show_credits` and the `print` defaults reach them at once through `render`.
- **Print page.** `/published/:id/print` (`PrintPage`, `PrintBody`, `lib/print.ts`). The options are in the query string (`variant`, `paper`, `lyrics`, `size`, and `readings`, `assignments`, `keys`, `notes` as `1` / `0`); an invalid value falls back to the church default. `@page` is `A4` or `215mm 330mm`, margin 15 mm. The menu, footer and controls are `print:hidden`; paper is always dark on white.
- **Musician sheet.** It always prints the first line of each section, whatever `lyrics` says, and the Lyrics control is hidden for it. Readings and assignments do not apply to it. The licence line is printed on it when credits are on; the copyright lines are not.
- **Readings off** prints the reference line and leaves out the reading text.
- **Header** date and time are printed as stored, without conversion.
- **Layout rules** are `break-after-avoid` on the song heading and notes, and `break-inside-avoid` on a heading with its first row and on every row of the sequence; tests check the classes, not page counts.
- **Tests.** TC `TestChurchPrintSettings`; `TestPrintSettings` (render, footer at publish, replace, clear, refusal, scope); IT-P-016 `TestPrintSettingsHTTP`; WT-P-004 (`PrintPage.test.tsx`), WT-P-005 (settings page); E2E-W-018. Showing credits on the musician sheet, ignoring `show_credits`, not copying the footer, and skipping the print validation each fails a test. Go on both dialects; Vitest 252; Playwright 38.
- **Not run:** golangci-lint (the v1/v2 config mismatch of step 3 still stands). The Indonesian texts of the Printing section, the sheets and the options are drafts for the owner.

### 11.5 Slice 5D as built (2026-10-05)

Merged as `05e8261`. Differences and additions to §7:

- **No Workbox.** The service worker is hand-written (`web/sw/sw.js`, about 140 lines) to avoid new dependencies. A Vite plugin (`serviceWorker` in `vite.config.ts`) fills in the build id (a hash of the built file names) and the precache list, and emits `dist/sw.js`. It is registered only in production builds (`lib/offline.ts`). The Go server serves `/sw.js`, `/manifest.webmanifest` and the two icons at the root with `Cache-Control: no-cache`; a missing one is a 404, not the index page.
- **Caches.** `shell-<build>` (cache first, plus the index page as the offline fallback for navigation) and `pub-<build>` (network first, 4 s timeout). Only `GET …/api/v1/liturgies/{id}/published` and `…/api/v1/me/assignments` without a query string are kept. The cache is opened only when an answer is stored, so a clear in between cannot be undone by a late answer.
- **Five liturgies.** The limit of 5 counts liturgies only; "my assignments" does not take a slot [Q-5.4]. A liturgy read again becomes the newest.
- **Clearing.** `clearOffline()` removes the marker, tells the worker (it bumps its generation and deletes `pub-*`), and deletes `pub-*` itself. **The shell cache stays**, since it holds only public files and the login page must open offline afterwards. It runs first at logout (also on the invite page), on a 401, and when `/me` shows a user other than the marker. A failing logout call is now ignored: offline, the server session stays until it expires (accepted; no queued logout).
- **Offline notice.** `lib/offline.ts` records the paths whose last answer carried `X-From-Cache: 1` (an `openapi-fetch` middleware); `OfflineNotice` shows it on the published view (with `published_at`) and on Home (without a date).
- **Opening offline.** When `GET /me` fails with a network error and the marker exists, `AppLayout` shows `OfflineFrame`: only "My assignments", the saved views they link to, and Log out. Reads are not retried while `navigator.onLine` is false.
- **Reading mode.** `?read=1` on `/published/:id` (`isReadingMode`); `AppLayout` drops the header and footer. The viewer's items (duty in `assignments[]` with their user ID) show "Your part" and a bar; "Go to my part" focuses the first. `KeepScreenOn` is a switch, hidden without the Wake Lock API.
- **Manifest and icons.** `web/public/manifest.webmanifest` and two placeholder icons (a white cross on blue) for the owner to replace. `index.html` links them.
- **Privacy.** The page gains "Phone numbers" and "Saved on your phone". The first already says approvers see numbers, which is true only once 5E is built.
- **Not built.** The church path prefix: the worker matches `/api/v1/…` at the root, and the "church path changes" clear is left for the SaaS step, as no prefix exists yet.
- **Tests.** WT-P-005 (`Reading.test.tsx`), WT-P-007 (`sw.test.ts` runs the worker with a fake cache; `Offline.test.tsx` for the client, logout and the offline frame), `TestFrontendBuilt` (root files), E2E-W-019. Dropping `X-From-Cache`, the trim, the clear, the route filter or the generation check each fails a test. Go on SQLite (no database change); Vitest 275; Playwright 39.
- **Not run:** golangci-lint. The Indonesian texts are drafts for the owner.

### 11.6 Slice 5E as built (2026-10-05)

Merged as `da99444`. Differences and additions to §8:

- **Summary route.** `GET /liturgies/{id}/published/summary` (`getPublishedSummary`, `liturgy.approve`, `private, no-store`). Besides the fields of §8 it returns `liturgy` (service name, date, time, language, church name) and `key_display`, which the texts need, and each recipient has `member` (false for a free-text name), so that the page can tell a member with no phone (Copy button) from a name with no account (no button). The items carry titles, duties, song headings and reading references only; a test checks that item text does not leak. A phone number appears in no other route.
- **Recipients.** One per assigned member of the newest version who is still a member (checked at read time; a member who left is skipped, and their phone is not read), one per free-text name (trimmed, case-folded), with their duty names.
- **`Compare`.** `domain.Compare(previous, latest)` returns `Changes{Items, Songs, Reading, Assignments}`. Songs match by song ID and occurrence within an item. A key change counts when the song's key or its ordered list of entry `key_change` values differs (`entry_keys` says which). A reading counts as changed when its reference or translation differs, or when it is added or removed on an item present in both versions. Duty changes compare IDs only. Moves use the longest common subsequence of the common items. `PublishedRepo.Previous` returns the version before the newest.
- **Texts.** `lib/messages.ts` builds them from the data with `i18next` fixed to the liturgy's language (`msg.*` keys), falling back to the church's default interface language for `zh-*`. The item list is the droppable block: the songs block and the readings lines for the team text; the person's parts and their songs and readings for a personal text; the change lines for the change summary. A title line left with nothing under it is dropped too. The check-boxes Songs, Keys and Readings also filter the change summary. The limit is 1,500 characters (not bytes).
- **Page.** `/published/:id/messages` (`MessagesPage`), linked from the published view and from the review bar for members with `liturgy.approve`. The preview is a read-only text area; Copy selects it when the Clipboard API refuses. The query is not kept after the page is left.
- **Link warning.** The server cannot tell whether `LITURGIST_BASE_URL` is set, so the page warns when the link's host is `localhost`, `127.0.0.1` or `::1`.
- **Tests.** TC-P-003 `TestCompare`; `TestSummaryOfPublished`; IT-P-010 `TestPublishedSummaryHTTP`; the `Published.Previous` harness entry (the harness seed now has two versions per church, so `Latest`, `Info`, `ListLatest` and `Upcoming` expect version 2); WT-P-006 (`messages.test.ts`, `Messages.test.tsx`); E2E-W-020. Removing the scope check, keeping a member who left, ignoring moves and ignoring the length limit each fails a test. Go on both dialects; Vitest 301; Playwright 40.
- **Not run:** golangci-lint. The Indonesian texts are drafts for the owner.

## 12. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Render the published view or print from the editable liturgy | Render the latest `PublishedVersion` only | Edits in progress must stay invisible; later library edits must not change what was distributed |
| Publish without `edit_seq`, or make the copy after the state update has committed | Require it; update state and insert the version in one transaction | A stale tab would publish unread content; a `published` liturgy without a version would show nothing |
| Delete with a read of the state and an unconditional `DELETE` | Condition on `state` and on the absence of versions; foreign key `NO ACTION` | A delete racing a publish, or of a reopened liturgy, would destroy the distributed copy |
| Store lyrics once per sequence entry | Store each used section once per item song | Repeats would multiply the size |
| Reuse `openLiturgy` for the published routes, or leave its `published` exception in place | A separate check (member and at least one version); remove the exception | The editable view and the edit history must not reach members without a scope |
| Block reopen because of cleared references | Allow it; show the problems | Owner decision 2026-10-05; the editor already replaces them |
| Read 100 content blobs to find a person's duties | `published_assignees`, then read the few matching versions | Cost on every page load of every team member |
| Put lyrics or Bible text in a WhatsApp text | Titles, numbers, keys, references, assignments, link | Copyright; SPEC §5.6 |
| Cache the user list, `/me`, `/church`, `summary` or any other route; keep the cache after logout | Only the two route patterns of §7; clear first at logout, on 401, on a different user | A shared phone must not show the last user's liturgies; `/me` holds e-mail and phone |
| Use `ETag` = `number` | `no-store` | Archive, reopen and settings change the response without changing `number` |
| Count a pixel size or a page count in a print test | Test the CSS rules and the option logic | Browsers differ; the test must not be flaky |
| Send a message or archive automatically | The user presses the button | SPEC §8.2.1, §5.6 |

## 13. Test case specifications

### Unit tests (domain)
| ID | Component | Input | Expected |
|---|---|---|---|
| TC-P-001 | Transition table | every (state, action) pair including `publish` and `reopen` from `published` | exactly the transition rows of §2 and §4 are allowed (archive and unarchive are not transitions: their predicates are tested in IT-P-007) |
| TC-P-002 | Content builder | a liturgy with a medley, repeated sections, a free-text item, an unassigned duty, a section with no label in an `id` liturgy viewed by an `en` user | sections stored once; entries refer to them; names as displayed; keys as letters; the derived label is in the liturgy's language |
| TC-P-003 | `Compare` | previous vs latest: item added / removed / retitled / duty changed, **one item inserted at the top**, two items swapped, song key changed, reading changed, person added / removed (member and free text, different spelling of a name), lyrics edited | exactly those changes; the insert at the top reports one addition and no moves; the swap reports one move; none for the lyrics; identical versions → empty arrays |
| TC-P-004 | Church print settings | every key, bad values, a partial `print` object | defaults when absent; 422 for a value outside the lists or a missing sub-key; `licence_footer` of 201 characters refused, the empty string accepted and clears |
| TC-P-005 | Message builder | all checkbox combinations, a very long liturgy, `id` and `en`, a Chinese liturgy, fixed blocks over the limit | no lyrics; whole item lines dropped from the bottom, "…" counted inside 1,500 runes; the fixed blocks never dropped; the language fallback; the warning |
| TC-P-006 | Print options | each option, the query string | the option logic of §6; invalid values fall back to the church default |
| TC-P-007 | Size measure | content with `<`, `>`, `&` and Chinese text around the cap | measured on the stored bytes (no HTML escaping); at the cap passes, one byte over → `publish_too_large` with `largest_item` |

### Integration tests (HTTP, both dialects)
| ID | Flow | Verification |
|---|---|---|
| IT-P-001 | Draft → … → approved → publish → reopen → edit → … → publish | versions 1 and 2 exist, immutable; the state changes are exact; the view shows `revising` between them |
| IT-P-002 | Publish with a stale `edit_seq`; two simultaneous publishes | 409 `review_stale`; exactly one 200, one version, one set of `published_assignees` |
| IT-P-003 | Forced failure inside the transaction (hook after the update, before the insert); a hook that makes the problems re-check fail | the liturgy stays `approved`; no version; `has_problems` 422 |
| IT-P-004 | Publish racing a **delete**, a **reopen** and a **library delete** of a song the liturgy uses (hooks, both interleavings) | delete: the liturgy and version 1 exist afterwards, the loser answers 404 or 409 `liturgy_not_deletable`; reopen: exactly one wins; library delete: refused while the liturgy is approved (`song_in_use`) |
| IT-P-005 | Delete a song used by the published liturgy; reopen; submit | the published copy still renders; reopen succeeds; the item shows `song_removed` with its title; submit → 422 `has_problems` until replaced |
| IT-P-006 | Limits: `max_unpublished_liturgies` at the edge; `max_active_liturgies` at the edge | reopen from `published` → 403 `limit_reached` at the edge with the unpublished message; unarchive likewise; archive never; reopen of an `approved` liturgy is not limited; 20 simultaneous reopens take the last slot once |
| IT-P-007 | Archive / unarchive / reopen of an archived liturgy; archive of a reopened one; archive twice; unarchive of an active one | `liturgy_archived`, `invalid_transition`, `not_archived` as in §4; archive and reopen racing: one wins |
| IT-P-008 | Visibility: team member with no scope, a member of another church, a liturgy with no version, **each in state `published` and in `draft`** | published routes 200 / 404 / 404; `GET /liturgies`, `/liturgies/{id}`, `/edits`, `/state-changes`, `/comments`, `/assignments` are 404 (list: empty) for the team member in every state (the `published` exception is gone) |
| IT-P-009 | "My assignments": two liturgies, an archived one, a past one, a liturgy dated today in the church's time zone around midnight UTC, a free-text name, a removed member, a revising liturgy, a user of another church | only the user's duties in upcoming non-archived liturgies, from the **published** assignments; `more` beyond 50; no other church's data |
| IT-P-010 | Leaks and caching headers: `summary` and the three read routes | phone and e-mail appear only in `summary` and only for members still in the church; 403 without `liturgy.approve`; all four answer `Cache-Control: private, no-store` with no `ETag`; an archive, a reopen and a print-settings change are visible on the very next GET |
| IT-P-011 | Archive-and-create | archives only valid published liturgies and creates the new ones atomically; a failure (unknown ID → 404, not published or too recent → 409, limit still exceeded → 403) leaves everything unchanged; needs both scopes (403 before any read) |
| IT-P-012 | Church print settings | saved through `PATCH /church`, returned in `render`; unknown keys kept; two simultaneous PATCHes keep each other's keys |
| IT-P-013 | Size cap | a realistic maximum liturgy publishes; a synthetic one over the cap → 422 `publish_too_large` and no state change |
| IT-P-014 | List parameters | `from`/`to`/`archived`/`limit`/`offset` and their 422s; order with equal date and time and with an empty time |
| IT-P-015 | Delete of a liturgy with a version | 409 `liturgy_not_deletable` in `published`, after a reopen (`draft`), and after an archive |

### Web (Vitest, `mockApi` and `renderPage`)
| ID | Subject | Verification |
|---|---|---|
| WT-P-001 | Review bar | Publish / Archive / Unarchive / Reopen per `actions`; the banners; the note field; the per-limit messages |
| WT-P-002 | Published view | Renders only the copy; keys in the church's display form; credits follow `show_credits`; the revising and archived notices |
| WT-P-003 | My assignments and navigation | Cards, empty state, the welcome text, the menu of §10 for a member with and without scopes; "A · A+ · A++" saves the preference |
| WT-P-004 | Print page | Every option changes the output; the query string is kept; musician variant shows no credits |
| WT-P-005 | Reading mode | My part highlighted with text, "Go to my part" moves focus, Wake Lock switch hidden without the API |
| WT-P-006 | Messages | The three texts for given data; checkboxes; no lyrics; Copy; WhatsApp links encoded; language of the liturgy |
| WT-P-007 | Offline | The notice appears when the response carries `X-From-Cache: 1`, with `published_at`; writes fail normally; logout clears the caches first, also when the API call fails; a different user ID clears; a late response after a clear is not stored (service worker faked) |
| WT-P-008 | i18n and accessibility | Every string through `publish.*` keys in `en` and `id` (Indonesian is a draft for the owner's review); live regions |

### End-to-end (Playwright)
E2E-W-017: the liturgist publishes an approved liturgy, a team member (no scope) opens "My assignments" and the published view, prints with the musician variant, opens reading mode; the liturgist reopens, republishes and sees the change summary; the team member sees the old version marked "being revised" in between and gets 404 on the editable liturgy's URL; axe passes on each page. E2E-W-019 (5D; E2E-W-018 is the print test of 5C): reading mode, then offline reload of the published view from the cache; logout clears it.

## 14. Error handling matrix

| Error | Code | Detection | User message (en) | Recovery |
|---|---|---|---|---|
| Wrong state for the action | 409 `invalid_transition` (`state`) | Conditional update changed no row | "This liturgy is now {{state}}." | Reload |
| Reviewed content changed | 409 `review_stale` | `edit_seq` differs | "The liturgy changed after you opened it. Read it again." | Reload |
| Unfinished items at publish | 422 `has_problems` | Re-check in the transaction | "Some items are not finished." | Reopen and fix |
| Archived | 409 `liturgy_archived` | `archived_at` set | "This liturgy is archived. Unarchive it first." | Unarchive |
| Not archived | 409 `not_archived` | `archived_at` null | "This liturgy is not archived." | Reload |
| Plan limit | 403 `limit_reached` (`limit`, `used`, `max`) | §4 | Unpublished: "Finish or delete an unpublished liturgy first." Active: "Archive another liturgy first." | As the message says |
| Cannot delete | 409 `liturgy_not_deletable` | Published, or any version exists | "A liturgy that has been published can only be archived." | Archive |
| Copy too large | 422 `publish_too_large` (`largest_item`) | Stored bytes over 4 MiB | "This liturgy is too large to publish. The longest item is {{title}}." | Shorten the longest texts |
| No published copy / not visible | 404 `not_found` | — | "This liturgy has not been published." | Back to the list |
| No permission | 403 `forbidden` | Missing scope | "You don't have permission to do this." | — |
| Invalid setting or parameter | 422 `validation_failed` | §5, §6 | "Check this field." | Fix and save |
| Offline | (client) | `X-From-Cache` header | "Offline — showing the last saved copy." | Reconnect |
| Newer content format | (client) | `format` > 1 | "Update the app to read this liturgy." | Reload |

## 15. References

| Topic | Location |
|---|---|
| Publishing, outputs, WhatsApp, reading mode | [SPEC.md §5.5](../SPEC.md#55-review-workflow), [§5.6](../SPEC.md#56-outputs), [§7](../SPEC.md#7-data-model-sketch), [§11](../SPEC.md#11-decisions-log) |
| Free-plan limits and archiving | [SPEC.md §8.2.1](../SPEC.md#821-usage-limits-saas-free-plan), [10 §8](10-liturgy.md#8-entitlements-and-limits) |
| Transitions, history, the undo floor, comments | [12 §2](12-review.md#2-states-and-transitions-p-68), [§4](12-review.md#4-comments-p-72) |
| Cleared references | [10 §6](10-liturgy.md#6-usage-ports-and-deleted-songs-readings-sections) |
| Prepare next week | [10 §3.1](10-liturgy.md#31-prepare-next-week-p-59) |
| Authorization and `actions`; church settings route | [04 §5](04-tenancy-extensions.md#5-authorization), [04 §6](04-tenancy-extensions.md#6-step-1-church-and-member-api) |
| Pages and navigation | [05 §2](05-web-shell.md#2-pages-in-step-1) |
| Pilot | [PILOT.md](../PILOT.md) |
| Tables | [reference/schema.md](../reference/schema.md#step-5-tables) |
