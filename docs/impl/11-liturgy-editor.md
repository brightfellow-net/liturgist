# 11 — Liturgy Pages, Live Updates and Undo (Implementation)

> **Document type: Implementation.** Step 3 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slices 3C (pages) and 3D (live updates, presence, undo and redo).
> Status: decisions **[P-xx] Approved** 2026-10-03 (after the adversarial review; conditions in the [index](README.md#4b-proposed-decisions-step-3)). Code may be written from this document. Items marked **[P-xx]** are decisions listed in the [index](README.md#4b-proposed-decisions-step-3). Nothing here is Approved until the owner says so.

## 1. Scope

Slice 3C is the web side of [09](09-planning.md) and [10](10-liturgy.md): the liturgy list, "Prepare next week", creating a liturgy, and the editor with its song sequences, readings and assignments. Slice 3D adds live updates, presence ("Budi is also editing"), and per-person undo and redo, on the history records of [10 §7](10-liturgy.md#7-history).

| Left out | Why | Where it lands |
|---|---|---|
| Comments on items, the review buttons (submit, approve, publish), state history | Step 4 | Step 4 |
| Published view, reading mode, "my assignments", print and PDF | Step 5 | Step 5 |
| Drag-and-drop reordering | Buttons with text are the only way to reorder in step 3 (P-52) | Not planned |
| Typing together in one text field | SPEC §5.2: version checks plus live updates, not co-editing | Not planned |

## 2. Key display [P-67]

The API stores keys as letters (`G`, `Bb`, `F#m`). The web app formats them with `formatKey(key, display)` using the church setting `key_display` ([03](03-identity-auth.md)): `letter` → `G`; `do` → `Do = G`. For a minor key in `do` mode it writes `La = Em` (the relative-major convention used with moveable do). The same function is used by every screen and, later, print and WhatsApp text. A key change entry reads "change to Do = A". The input always accepts and shows letters; the formatted text is a label next to it.

## 3. Pages (slice 3C)

All text through i18n ([05 §6](05-web-shell.md#6-translations)); tap targets, text size and reflow as in [05 §7](05-web-shell.md#7-accessibility-and-text-size); the pages must work at 320 px width and 200% text. The menu item and tabs are described in [09 §5](09-planning.md#5-pages).

| Route | Page | Who |
|---|---|---|
| `/liturgies` | **Upcoming** (date today or later, oldest first) and **Past** (newest first) tabs; each row: date, time, service name, language, state in words, number of items, link. Buttons "Prepare next week" and "New liturgy" for `liturgy.edit`. Empty state: explains liturgies and offers both buttons; if the church has no services it also links to the Services page | Liturgy visibility ([10 §2.1](10-liturgy.md#21-liturgy)) |
| `/liturgies/prepare` | Week picker (a date input and "Previous week" / "Next week" buttons) showing "Week of 12 October 2026"; the occurrences as a checklist (service, weekday, date, time, template, language), all ticked except slots that already have a liturgy (disabled, with a link "Open"); when a limit applies, a sentence "N of M slots free" and the button disabled with an explanation if too many are ticked; button "Create N liturgies" → the list. With no services, an explanation and a link | `liturgy.edit` |
| `/liturgies/new` | Form: choice "Regular service" / "One-off service"; service (list) or name; date; time; template (list, preselected from the service); language (preselected); button "Create". `liturgy_exists` shows the link to the existing liturgy | `liturgy.edit` |
| `/liturgies/{id}` | The editor (§4); read-only when `actions.edit` is false | Visibility |

### 3.1 List API additions

`GET /liturgies` also takes `order` (`date_asc` or `date_desc`, default `date_desc`); the Upcoming tab sends `from=<today in the church's time zone>&order=date_asc`, the Past tab `to=<yesterday>`. `from` and `to` are inclusive dates.

## 4. The editor

One page, a column of cards; no modal dialogs (confirmations are inline as in `ConfirmButton`).

1. **Header:** service name, date, time, language, state in words; "Edit details" (name, date, time) with its own Save; a notice "Locked: this liturgy is in review" when `actions.edit` is false.
2. **Problems:** the list from `problems` in words with a link to the item ("Item 3 has no song yet"); hidden when empty.
3. **Items**, in order. Each item is its own form with its own **Save item** button, a "Saved" confirmation, and an unsaved-changes marker:
   - Title, duty (list), and **Move up / Move down / Remove** (Remove asks inline).
   - `prayer`, `sermon`, `free_text`, `other`: a text box.
   - `reading`: the chosen reading (reference, translation, text), "Choose a reading" (search of saved readings by `GET /readings?q=`, with a link to add a new one), "Clear".
   - `song`: the item's songs in order, each with **Move up / Move down / Remove**, key (input plus the formatted label), note, and its **sequence**: rows of section (list showing "Verse 1", "Chorus"…, with "Show lyrics" to read the text), singing part (list, optional), key change (input), note, each with Move up / Move down / Remove; "Add an entry"; "Fill from the default order" restores what adding the song gave. "Add a song" searches the library (`GET /songs?q=`), shows hymnal number and language, and adds it with its sequence filled by the server.
4. **Add an item:** title, type, duty, then "Add"; it is added at the end and can be moved. A hint says the type cannot be changed later.
5. **Team:** for each duty that has people or an item using it: the names, "Remove" per person, and **Add a person** (a list of members from `GET /liturgies/assignable`, or "Someone without an account" with a name box).
6. **Recent changes:** the last 20 `liturgy_edits` in words ("Budi moved Pujian up", "Ruth changed the key of Besar Setia-Mu"), with the time. Slice 3D adds undo and redo here.

**Saving and conflicts:** each item form keeps its own copy of the item's `version`; **Save item** sends it. On 409 `version_conflict` (`scope: "item"`) the card shows "This item was changed by someone else", keeps what the user typed, and offers **Show their version** (reloads that item's saved values beside the typed ones, as "Reload" does on the song form) and **Keep mine and save again** (re-sends the typed values with the new version). A `scope: "liturgy"` conflict (order, add, remove) reloads the item list and says so in a status line; unsaved item text is kept. Structure buttons (Move, Add, Remove) save at once with the liturgy version, update the list from the response, and announce the result in a polite live region ("Pujian moved to position 3 of 7"); focus stays on the button that was used.

**Accessibility:** every control has a visible text label; the sequence rows are fieldsets with a legend ("Entry 3: Chorus"); moving uses buttons with text, never drag alone; no information by colour alone (the unsaved marker has text); axe covers every state listed in §8.

## 5. Web code layout

New files under `web/src/routes/liturgy/` (pages and editor parts), `web/src/lib/liturgy.ts` (queries, `formatKey`, limits), and the API client types; query keys `["liturgies", filters]`, `["liturgy", id]`, `["templates"]`, `["services"]`, `["duties"]`, `["singing-parts"]`, `["prepare", week]`. A saved item updates `["liturgy", id]` from the response and does not refetch the whole liturgy.

## 6. Ports for slice 3D

| Port | Methods | Implemented by |
|---|---|---|
| `EventBus` | As step 1 ([04 §7](04-tenancy-extensions.md#7-extension-points)); topic `liturgy:<church_id>:<liturgy_id>` | In-memory (community) |
| `ChurchStore.Edits()` | `NextSeq(liturgy)`, `Append`, `Newest(user, liturgy, status, window)`, `ForeignTouches(liturgy, afterSeq, user, target)`, `SetStatus` (conditional), `DropUndone(user, liturgy)` | `adapters/sqlstore` |
| Presence registry | In-memory map, no port: derived from `EventBus` messages | `server` |

## 7. Live updates and undo (slice 3D)

### 7.1 Server-sent events [P-66]

`GET /liturgies/{id}/events` (a visible liturgy; `text/event-stream`):

- Authorization is the same as `GET /liturgies/{id}`, checked when the stream opens and again every 60 seconds; if the member no longer qualifies the server sends `event: closed` and ends the stream. 401/404 as for the liturgy. **Stated bound:** after a permission or membership change a stream may keep delivering IDs, versions and the actor's name for up to 60 seconds. This is accepted because events never carry content and every re-read goes through the checked routes. A removed member's open streams are also closed at once by the member-removal use case (it publishes to the church's topic), so the 60 seconds apply to scope changes only.
- The server sends a comment line (`: keepalive`) every 20 seconds and uses `http.ResponseController` to extend the write deadline, so the server's global timeouts do not cut the stream; responses have `Cache-Control: no-store` and `X-Accel-Buffering: no`.
- **Events carry IDs and versions only, never text** — a client re-reads what it needs through the normal, authorized routes:

| `event` | `data` (JSON) |
|---|---|
| `changed` | `{ kind: "liturgy" \| "items" \| "item" \| "assignments", item_id?, version, actor: { id, name } }` |
| `presence` | `{ users: [{ id, name }] }` (everyone with an open page, including the receiver) |
| `closed` | `{}` |

- A client ignores `changed` events whose `actor.id` is its own user **and** whose `version` it already holds. For a clean card it re-reads the item silently; for a card with unsaved input it shows "Changed by Budi" and does nothing else (the user's input is never replaced).
- On connect or reconnect the browser re-reads the whole liturgy once (events are not replayed; there is no `Last-Event-ID`).
- Limits: 5 open streams per member per liturgy, 500 per server. A sixth stream of one member is **refused** (429 `too_many_streams`, with `Retry-After: 30`), not served by closing the oldest: closing the oldest would make an open page reconnect and evict another in a loop. The page that gets the 429 shows "This liturgy is open in too many tabs. Close one and reload." and does **not** reconnect by itself. The 500-per-server cap answers 503 `unavailable` with `Retry-After: 30`, and the page retries with the backoff of the next line. A stream that ends for any other reason is reopened by the browser after 3 s, then 6 s, 12 s … up to 60 s, with random jitter, and the counter is reset after a stream stayed open for a minute.

**Presence:** presence is tracked **per connection**, not per user. Each open stream has a random `conn_id` and publishes `presence.hello { user, conn_id }` to the liturgy's topic every 20 seconds; each server keeps `conn_id → (user, last seen)` per liturgy and drops entries older than 60 seconds. A closed stream publishes `presence.bye { conn_id }`, which removes **only that connection**; a user is present while at least one of their connections is. `hello` for a known `conn_id` only refreshes it and `bye` for an unknown one is ignored, so duplicates and reordered messages are harmless (a late `hello` after a `bye` revives the connection until it expires in 60 seconds at most). The list in the `presence` event is the set of distinct users, sent when it changes. This path is the same with the in-memory bus and with a shared bus. The web app shows "Budi is also editing" (several: "Budi and Ruth are also editing"), never the viewer's own name.

### 7.2 Undo and redo [P-65]

Per person, per liturgy, on the history rows of [10 §7](10-liturgy.md#7-history). Rows of editing commands have a `status`: `done`, `undone` (with `undo_seq`, the `seq` of the `undo` row that undid it), or `dropped` (an undone edit that can no longer be redone). Undo and redo are themselves history rows (`undo`, `redo`, with `target_edit_id`), never targets, never part of the window.

| Method & path | Request | Response |
|---|---|---|
| `POST /liturgies/{id}/undo` | `{}` | The edit that was undone and the new versions; or 409 `undo_refused` |
| `POST /liturgies/{id}/redo` | `{}` | The edit that was redone; or 409 `undo_refused` |

Both need `liturgy.edit` and an editable state, and run in one `Tx.Write` that takes the history counter first ([10 §5](10-liturgy.md#5-versions-and-conflicts-p-60)), so the test below sees every committed edit.

- **Undo target:** among the caller's last 50 *editing* rows in this liturgy (`undo` and `redo` rows do not count), the newest with `status = 'done'`. None → 409 `undo_refused`, `reason: "nothing_to_undo"`. `liturgy.create` is never a target.
- **Condition ("nobody else changed it since"):** the test is on **other people's rows**, not on version numbers, because the caller's own undos and redos raise the versions. The target is refused with `reason: "changed_since"` (and nothing changes) when a row exists in this liturgy with `seq` greater than the target's, written by **another user**, that *touches* the target:

| Target command | A foreign row touches it when |
|---|---|
| `item.update`, `item.songs` | its `item_id` is the same item (any command, including that person's own undo or redo) |
| `item.add`, `item.remove` | its `item_id` is the same item, **or** it is structural (`liturgy.update`, `item.add`, `item.remove`, `items.reorder`, or an undo/redo of one of those) |
| `items.reorder`, `liturgy.update` | it is structural |
| `assignment.add`, `assignment.remove` | it is an assignment row (or its undo/redo) for the same duty and the same person |

  The caller's *own* later rows never refuse: the target is their newest `done` edit, and their later undone or dropped edits have already been reversed. This is what lets a person undo several steps in a row: after two edits A1, A2 of one item, undoing A2 and then A1 both pass (the version of the item has long passed `A1`'s, which is why versions are not the test), while a colleague's save of that item between A1 and A2 refuses the undo of A1. You can undo your move of an item after someone else edited another item's text, but not after someone edited that item. The test is the same on both dialects: one read of the history rows with `seq` above the target's.
- **Effect:** the `before` image is applied through the same repository calls as a normal write (versions rise as for any change, from their current values; the conditional update on the version just read means that a concurrent writer who got in is reported as `changed_since`, not overwritten), the edit becomes `undone` with `undo_seq` = the new `undo` row's `seq`, an `undo` row is appended, and `changed` events are published. An item removal is undone by re-creating the item with its **old ID**, `version = image.version + 1` and its position ([10 §7](10-liturgy.md#7-history)); if the image refers to a duty, reading, song, section or part that no longer exists → `reason: "reference_gone"`, nothing changes.
- **Redo target:** among the caller's `undone` edits, the one with the highest `undo_seq` (the most recently undone, so a stack of undos is redone in reverse order). The same condition as above, with foreign rows counted after the target's `undo_seq` — an edit by someone else made since the undo refuses the redo. Effect: the `after` image is applied; a `redo` row is appended; the edit becomes `done` again and `undo_seq` is cleared.
- **Any new editing command** by a person (not undo or redo) turns all of their `undone` edits in that liturgy into `dropped`, inside the new edit's transaction, as in an ordinary editor.
- A locked liturgy → 409 `liturgy_locked`.

**Worked example** (one item X; A = the caller, B = a colleague): A1 A edits X's text (`seq` 5), A2 A edits X again (6), B edits another item Y (7). Undo → A2 (no foreign row touches X) → `done`→`undone`, `undo` row (8). Undo → A1 (foreign rows after 5: B's 7, on Y; A's own 6 and 8) → passes. Redo → A1 (highest `undo_seq` is 9), then redo → A2. Had B saved X at `seq` 7, the first undo is refused with `changed_since`, and A's `done` stack stays as it is.

The web app shows **Undo** and **Redo** buttons with text in "Recent changes", disabled when the last response said nothing applies, and binds Ctrl/Cmd+Z and Ctrl/Cmd+Shift+Z (and Ctrl+Y) **only when the focus is not in a text field** (inside a field the browser's own text undo is used). The result is announced ("Undid: moved Pujian") and a refusal is explained ("Someone has changed this item since, so it can't be undone").

## 8. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-E-001 | `formatKey` | `G`, `Bb`, `F#m`, `` in both display modes | `G` / `Do = G`; `Bb` / `Do = Bb`; `F#m` / `La = F#m`; empty → empty | Unknown text is shown as typed |
| TC-E-002 | Item form state | Edit, save, conflict, "Show their version", "Keep mine" | Typed input is never lost; the version used is the one last loaded | Conflict while another card is dirty |
| TC-E-003 | Sequence editor | Move, remove, add entry, fill from default order | Order and fields as expected; only sections of the song are offered | 100 entries: "Add an entry" disabled |
| TC-E-004 | Prepare page | Week with 3 occurrences, one existing; limit with 2 slots left | Existing disabled; Create button disabled when 3 are ticked and explained | Week change refetches; no services |
| TC-E-005 | Event handling | `changed` for a clean card, a dirty card, own action, `presence` | Silent reload / notice only / ignored / names shown | Closed stream shows a reconnect message |
| TC-E-006 | Undo keys | Ctrl+Z in a text field and outside | Browser undo / server undo | Refusal message per `reason` |
| TC-U-001 | Undo conditions (Go) | Each command of [10 §7](10-liturgy.md#7-history), undone right away | The `before` state, versions raised, status `undone`, an `undo` row | Item add: undone after its text was edited by someone else → `changed_since`; by the caller → allowed |
| TC-U-002 | Undo stacks (Go) | Two users alternating edits; one user makes A1, A2, A3 on one item and undoes three times | Each undoes only their own newest edit; **three undos in a row pass** although each raised the versions; a new edit drops redo | 50-edit window counts editing rows only |
| TC-U-003 | Redo conditions (Go) | Undo, undo, redo, redo; undo then foreign edit then redo | Redone in reverse order (highest `undo_seq` first); `changed_since` | Item removal redo after the duty was deleted → `reference_gone` |
| TC-U-004 | Touch table (Go) | Every row of the table in §7.2 with a foreign row that does and does not touch, before and after the target | Refused exactly when the table says | Foreign undo/redo rows count; the caller's own rows never do |
| TC-U-005 | Restoring a removed item | Remove an item with songs, undo, redo, undo | Same ID, `version` = old + 1 and rising, position clamped, the `undo` and `redo` rows are not targets | Liturgy shrank meanwhile |
| TC-U-006 | Presence (Go) | Two connections of one user; `bye` of one; `hello` twice; `bye` before `hello`; expiry | The user stays until the last connection goes; duplicates harmless | Reordered messages |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-E-001 | Undo over HTTP | Liturgy with items; two sessions | Undo/redo for add, remove, move, edit, songs, assignment; the other session's change to the same item → 409 `undo_refused`; to another item → allowed | — |
| IT-E-002 | Events | Two sessions on one liturgy | The second receives `changed` with IDs and versions only (no text), then `presence` listing both; a member who loses `liturgy.edit`'s visibility gets `closed` within 60 s; the 6th stream of a member → 429 `too_many_streams` with `Retry-After`, and the first five stay open | — |
| IT-E-003 | Race | Parallel undo and edit of one item (race harness) | Exactly one wins; no lost update; versions consistent on both dialects | — |
| IT-E-004 | Locked and gone | Liturgy set to `in_review`; item deleted by another user | Undo → `liturgy_locked`; redo/undo of a deleted item → `changed_since` or `reference_gone`, never a 500 | — |

### Web end-to-end

| Test ID | Flow |
|---|---|
| E2E-W-012 | Add a duty, a singing part, a template with three items and a service (slice 3A); conflict message when a second session saved first |
| E2E-W-013 | Create a liturgy from the template; edit text; add a reading item and choose a saved reading; add a song (sequence filled from its arrangement), reorder entries, set a part and a key change; assign a member and a free-text name; reload and everything is there; the library refuses to delete the song while the liturgy exists |
| E2E-W-014 | "Prepare next week": two services, one slot already existing; create; the list shows them |
| E2E-W-015 | Two sessions: both edit different items; both edit one item and the second sees the conflict with its text kept; the first sees "Ruth is also editing" and the live change; undo and redo with the buttons and with Ctrl+Z; undo refused after the other session's change |
| E2E-W-005 | axe on the list, prepare, new, editor (several items, with a conflict shown and a locked liturgy), and the planning pages; the editor also fits 320 px without sideways scrolling |

## 9. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Send item text or names of songs in events | IDs, versions and the actor only | Events are a signal; content goes through authorized routes |
| Replace what a user typed when another person's change arrives | Show a notice and keep the input | SPEC §5.2: never lose edits silently |
| Make Ctrl+Z always call the server | Only outside text fields | Text fields must keep their native undo |
| Undo "the latest edit of the liturgy" | The caller's own latest edit, refused if **another person's** row touched it since | SPEC §5.2: one person must not undo another's work |
| Compare version numbers to decide whether an undo is allowed | Compare history rows (`seq`) written by other people | The caller's own undos raise the versions, so a version test refuses the second undo |
| Track presence per user and remove the user on one `bye` | Per `conn_id`; a user stays while any connection lives | Two tabs would make the user vanish while still editing |
| Hold a lock while someone edits | Versions and live notices | Stale locks block a team on Sunday morning |
| Rely on the stream for correctness | Re-read on connect; the version check decides | Streams drop (phones, proxies); the data must stay right without them |
| Reorder with drag and drop only | Buttons with text (P-52) | Older users, keyboards, screen readers |
| Use colour only for "unsaved" or "locked" | Text as well | WCAG 1.4.1 |
| Cut the event stream with the server's global write timeout | Per-write deadlines through `ResponseController` | Otherwise every stream dies after the timeout |
| Show the viewer's own name as "also editing" | Exclude the viewer | Confusing, especially with two tabs |

## 10. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| Item `version_conflict` | 409 `scope: "item"` | "This item was changed by someone else." | Show their version / keep mine |
| Liturgy `version_conflict` | 409 `scope: "liturgy"` | "The items were changed by someone else. The list was reloaded." | Repeat the action |
| `undo_refused` | 409, `reason` | `nothing_to_undo`: "Nothing to undo." `changed_since`: "Someone has changed this since, so it can't be undone." `reference_gone`: "Something this change used no longer exists." | — |
| `liturgy_locked` | 409 | "This liturgy can't be edited now." | Reload |
| Stream lost | `EventSource` error | "Live updates stopped. Reconnecting…" (status line) | Automatic; the page re-reads on reconnect |
| `too_many_streams` | 429, `Retry-After` | "This liturgy is open in too many tabs. Close one and reload." | The page does not reconnect by itself |
| `unavailable` (server stream cap) | 503, `Retry-After` | "Live updates are busy. Trying again shortly." | Automatic, with backoff and jitter ([§7.1](#71-server-sent-events-p-66)) |
| `forbidden`, `not_found` | 403, 404 | As in [10 §12](10-liturgy.md#12-error-handling-matrix) | — |

## 11. References

| Topic | Location |
|---|---|
| Concurrent editing, undo | [SPEC.md §5.2](../SPEC.md#52-weekly-liturgy), [§11](../SPEC.md#11-decisions-log) (concurrent editing and per-person undo rows) |
| Data and API | [09](09-planning.md), [10](10-liturgy.md), [reference/schema.md](../reference/schema.md#step-3-tables) |
| `EventBus` | [04 §7](04-tenancy-extensions.md#7-extension-points) |
| Web shell, accessibility, translations | [05](05-web-shell.md) |
| Song and reading pickers | [06 §3](06-song-library.md#3-api), [07 §4](07-readings.md#4-api) |
