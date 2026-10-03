# 11 — Liturgy Pages and Undo (Implementation)

> **Document type: Implementation.** Step 3 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slices 3C (pages) and 3D (undo and redo; live updates and presence are on the icebox, [§7.1](#71-live-updates-and-presence-icebox-p-66)).
> Status: decisions **[P-xx] Approved** 2026-10-03 (after the adversarial review; conditions in the [index](README.md#4b-proposed-decisions-step-3)). Code may be written from this document. Items marked **[P-xx]** are decisions listed in the [index](README.md#4b-proposed-decisions-step-3). Nothing here is Approved until the owner says so.

## 1. Scope

Slice 3C is the web side of [09](09-planning.md) and [10](10-liturgy.md): the liturgy list, "Prepare next week", creating a liturgy, and the editor with its song sequences, readings and assignments. Slice 3D adds per-person undo and redo, on the history records of [10 §7](10-liturgy.md#7-history).

| Left out | Why | Where it lands |
|---|---|---|
| Comments on items, the review buttons (submit, approve, publish), state history | Step 4 | Step 4 |
| Published view, reading mode, "my assignments", print and PDF | Step 5 | Step 5 |
| Drag-and-drop reordering | Buttons with text are the only way to reorder in step 3 (P-52) | Not planned |
| Typing together in one text field | SPEC §5.2: version checks, not co-editing | Not planned |
| Live updates and presence ("Budi is also editing") | Owner decision 2026-10-03: saves development time; the version checks already prevent lost edits | Icebox, [§7.1](#71-live-updates-and-presence-icebox-p-66) |

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

**As built (slice 3C).** Differences from the text above:

- A reading chosen in the picker is part of the item's draft and is saved with **Save item**, not at once.
- "Add a song" saves at once, but only while the item's song list has no unsaved edits; otherwise the card says to save first. **Save item** on a song item sends up to two requests (the item fields, then the complete song list); if the second fails, the first stays saved.
- A locked liturgy is shown read-only (cards without forms). Its look is covered by a component test only: no route in step 3 can put a liturgy into review, so there is no end-to-end or axe check of it until step 4.
- The new-liturgy form offers only templates of the chosen language and sends no template when the preselected one does not match.
- English dates read "12 October 2026". An ordinary item edit appears in Recent changes as "changed X"; only a key change names the key.
- The item-conflict flow has its own end-to-end test (E2E-W-013, second test); E2E-W-015 in slice 3D covers the same flow with two sessions.
- The Indonesian `liturgy.*` wording is a draft for the owner's review.

## 5. Web code layout

New files under `web/src/routes/liturgy/` (pages and editor parts), `web/src/lib/liturgy.ts` (queries, `formatKey`, limits), and the API client types; query keys `["liturgies", filters]`, `["liturgy", id]`, `["templates"]`, `["services"]`, `["duties"]`, `["singing-parts"]`, `["prepare", week]`. A saved item updates `["liturgy", id]` from the response and does not refetch the whole liturgy.

## 6. Ports for slice 3D

| Port | Methods | Implemented by |
|---|---|---|
| `ChurchStore.Edits()` | `NextSeq(liturgy)`, `Append`, `Newest(user, liturgy, floor, window)`, `ForeignTouches(liturgy, afterSeq, user, target)` (joins an undo/redo row to its target through `target_edit_id`), `SetStatus` (conditional), `MarkSkipped(edit)`, `DropUndone(user, liturgy)` | `adapters/sqlstore` |

## 7. Undo and redo (slice 3D)

### 7.1 Live updates and presence (icebox) [P-66]

**On the icebox since 2026-10-03 (owner decision): not built in step 3.** To save development time, slice 3D is undo and redo only. There is no event stream, no presence ("Budi is also editing") and no "Changed by Budi" notice. Concurrent editing is protected by the versions and the conflict screen of slice 3C ([10 §5](10-liturgy.md#5-versions-and-conflicts-p-60)); another person's change shows up when the page is reloaded or when a save conflicts. Nothing in 3C or 3D depends on the stream, and undo and redo work without it.

What the design (kept in git history, `docs/impl/11-liturgy-editor.md` at commit `6d5ea45`) decided, so a later start does not begin from zero:

- `GET /liturgies/{id}/events`, server-sent events carrying IDs and versions only, never text; presence tracked per connection (`conn_id`) from heartbeats and expiring after 60 seconds; authorization re-checked every 60 seconds; 5 streams per member per liturgy (a sixth is refused with 429, not served by closing another); the page re-reads the liturgy on every (re)connect.
- **Spike result (2026-10-03, throwaway code, not in the repository):** a stream with a 20-second keepalive and per-write deadlines through `http.ResponseController` stayed open for 130 seconds behind nginx (default settings, gzip on, HTTP/1.1 upstream) and Caddy (`encode gzip zstd`), with events delivered on time. Without the deadline extension a 30-second global `WriteTimeout` cut the stream at 40 seconds. The `X-Accel-Buffering: no` header made no difference in this test (events were small and 45 seconds apart). Not tested: Cloudflare Tunnel, Tailscale, HTTP/2 to the browser.
- Anything that adds a `WriteTimeout` to the server must keep this in mind when streaming returns.

### 7.2 Undo and redo [P-65]

Per person, per liturgy, on the history rows of [10 §7](10-liturgy.md#7-history). Rows of editing commands have a `status`: `done`, `undone` (with `undo_seq`, the `seq` of the `undo` row that undid it), or `dropped` (an undone edit that can no longer be redone), and a flag `skipped` (a `done` edit whose undo was refused and that is no longer offered, below). Undo and redo are themselves history rows (`undo`, `redo`, with `target_edit_id` and the `item_id` of their target), never targets, never part of the window. The touch test needs the *command* of the edit an undo or redo row acted on: it reads it through `target_edit_id` (one join inside the same liturgy), so no column repeats it.

| Method & path | Request | Response |
|---|---|---|
| `POST /liturgies/{id}/undo` | `{}` | The edit that was undone and the new versions; or 409 `undo_refused` |
| `POST /liturgies/{id}/redo` | `{}` | The edit that was redone; or 409 `undo_refused` |

Both need `liturgy.edit` and an editable state, and run in one `Tx.Write` that takes the history counter first ([10 §5](10-liturgy.md#5-versions-and-conflicts-p-60)), so the test below sees every committed edit.

- **Undo target:** among the caller's last 50 *editing* rows in this liturgy with `seq` above the liturgy's `undo_floor_seq` (`undo` and `redo` rows do not count), the newest with `status = 'done'` that is not `skipped`. None → 409 `undo_refused`, `reason: "nothing_to_undo"`. `liturgy.create` is never a target.
- **Floor [P-65, review 2026-10-03]:** `liturgies.undo_floor_seq` (default 0) is set by every state change of the review workflow (step 4: submit, approve, send back, publish, reopen) to the liturgy's `edit_seq` at that moment. Rows at or below the floor are neither undo nor redo targets and do not count in the window, so nobody undoes an edit a reviewer has already read. Slice 3D only reads the column; step 4 writes it.
- **Condition ("nobody else changed it since"):** the test is on **other people's rows**, not on version numbers, because the caller's own undos and redos raise the versions. The target is refused with `reason: "changed_since"` (and nothing changes) when a row exists in this liturgy with `seq` greater than the target's, written by **another user**, that *touches* the target:

| Target command | A foreign row touches it when |
|---|---|
| `item.update`, `item.songs` | its `item_id` is the same item (any command, including that person's own undo or redo) |
| `item.add`, `item.remove` | its `item_id` is the same item, **or** it is structural (`liturgy.update`, `item.add`, `item.remove`, `items.reorder`, or an undo/redo of one of those) |
| `items.reorder`, `liturgy.update` | it is structural |
| `assignment.add`, `assignment.remove` | it is an assignment row (or its undo/redo) for the same duty and the same person |

  **A refused undo does not trap the older edits [P-65, review 2026-10-03].** If the refused target is *not structural* (`item.update`, `item.songs`, `assignment.add`, `assignment.remove`), the use case, after rolling back its own transaction (so `seq` has no gap), sets `skipped = true` on that row in a second short `Tx.Write` and then answers 409 `changed_since`; the next undo takes the next older `done` edit. The flag is safe to keep because a touch only ever grows: the edit could never become undoable again. A refused **structural** edit (`item.add`, `item.remove`, `items.reorder`, `liturgy.update`) is *not* skipped and keeps blocking: restoring a removed item by its old position is exact only when every later structural edit of the same person has been reversed first (a model of these rules found position errors when such an edit was skipped).

  The caller's *own* later rows never refuse: the target is their newest `done` edit, and their later undone or dropped edits have already been reversed. This is what lets a person undo several steps in a row: after two edits A1, A2 of one item, undoing A2 and then A1 both pass (the version of the item has long passed `A1`'s, which is why versions are not the test), while a colleague's save of that item between A1 and A2 refuses the undo of A1. You can undo your move of an item after someone else edited another item's text, but not after someone edited that item. The test is the same on both dialects: one read of the history rows with `seq` above the target's.
- **Effect:** the `before` image is applied through the same repository calls as a normal write (versions rise as for any change, from their current values; the conditional update on the version just read means that a concurrent writer who got in is reported as `changed_since`, not overwritten), the edit becomes `undone` with `undo_seq` = the new `undo` row's `seq`, and an `undo` row is appended. An item removal is undone by re-creating the item with its **old ID**, `version = image.version + 1` and its position ([10 §7](10-liturgy.md#7-history)); if the image refers to a duty, reading, song, section or part that no longer exists, or (assignments) to a member who has left the church → `reason: "reference_gone"`, nothing changes.
- **Versions never go back [P-65, review 2026-10-03].** An item that is re-created (undo of a removal, redo of an add) gets `version` = the version the item had in the image of the row that removed it, plus 1: for an `item.remove` row that is its `before` image; for an `undo` or `redo` row that removed it, its `before` image (which therefore holds the complete item). Using the `after` image of the original `item.add` would give version 2 to an item that had reached 3 through undone edits, and a stale tab could then save over it.
- **Redo target:** among the caller's `undone` edits, the one with the highest `undo_seq` (the most recently undone, so a stack of undos is redone in reverse order). The same condition as above, with foreign rows counted after the target's `undo_seq` — an edit by someone else made since the undo refuses the redo. A refused redo makes the caller's whole undone stack useless (the top can never become valid again, and redoing older ones around it is not safe), so after rolling back, a second `Tx.Write` sets all of the caller's `undone` edits in this liturgy to `dropped`, and the answer is 409 `changed_since`. Effect: the `after` image is applied; a `redo` row is appended; the edit becomes `done` again and `undo_seq` is cleared.
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
| TC-E-005 | Event handling | — | Iceboxed with the live updates ([§7.1](#71-live-updates-and-presence-icebox-p-66)); the ID is not reused | — |
| TC-E-006 | Undo keys | Ctrl+Z in a text field and outside | Browser undo / server undo | Refusal message per `reason` |
| TC-U-001 | Undo conditions (Go) | Each command of [10 §7](10-liturgy.md#7-history), undone right away | The `before` state, versions raised, status `undone`, an `undo` row | Item add: undone after its text was edited by someone else → `changed_since`; by the caller → allowed |
| TC-U-002 | Undo stacks (Go) | Two users alternating edits; one user makes A1, A2, A3 on one item and undoes three times | Each undoes only their own newest edit; **three undos in a row pass** although each raised the versions; a new edit drops redo | 50-edit window counts editing rows only |
| TC-U-003 | Redo conditions (Go) | Undo, undo, redo, redo; undo then foreign edit then redo | Redone in reverse order (highest `undo_seq` first); `changed_since` | Item removal redo after the duty was deleted → `reference_gone` |
| TC-U-004 | Touch table (Go) | Every row of the table in §7.2 with a foreign row that does and does not touch, before and after the target | Refused exactly when the table says | Foreign undo/redo rows count; the caller's own rows never do |
| TC-U-005 | Restoring a removed item | Remove an item with songs, undo, redo, undo; add an item, edit it, undo the edit, undo the add, redo the add | Same ID, `version` = the version it had when removed + 1 and rising (never a value it had before), position clamped, the `undo` and `redo` rows are not targets | Liturgy shrank meanwhile |
| TC-U-007 | Undo model (Go) | Random histories of two users over the full command set, with undo and redo, replayed through the real use cases on both dialects; the oracle is the model of the 2026-10-03 review (state = replay of the `done` edits only) | After every step the state equals the replay; refusals are only `changed_since` / `nothing_to_undo` / `reference_gone`; no version repeats | Seeds fixed in the test; a failure prints the history |
| TC-U-008 | Skips and floor | A non-structural refusal then an older undo; a structural refusal then an older undo; a redo refusal; a state change that sets the floor | The older edit is offered after a non-structural refusal; a structural refusal keeps blocking; a refused redo drops the stack; rows at or below the floor are no targets | The skip survives a reload (it is stored) |
| TC-U-006 | Presence (Go) | — | Iceboxed with the live updates; the ID is not reused | — |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-E-001 | Undo over HTTP | Liturgy with items; two sessions | Undo/redo for add, remove, move, edit, songs, assignment; the other session's change to the same item → 409 `undo_refused`; to another item → allowed | — |
| IT-E-002 | Events | — | Iceboxed with the live updates; the ID is not reused | — |
| IT-E-003 | Race | Parallel undo and edit of one item (race harness), repeated at least 100 times on PostgreSQL | Exactly one wins; no lost update; versions consistent on both dialects; a PostgreSQL deadlock is absorbed by the retry of [02 §3](02-persistence.md#3-connections-and-transactions), never a 500. Undo locks the liturgy row first and then the item row, an item edit the other way round ([10 §5](10-liturgy.md#5-versions-and-conflicts-p-60)); this is why the test must run there | — |
| IT-E-004 | Locked and gone | Liturgy set to `in_review`; item deleted by another user | Undo → `liturgy_locked`; redo/undo of a deleted item → `changed_since` or `reference_gone`, never a 500 | — |

### Web end-to-end

| Test ID | Flow |
|---|---|
| E2E-W-012 | Add a duty, a singing part, a template with three items and a service (slice 3A); conflict message when a second session saved first |
| E2E-W-013 | Create a liturgy from the template; edit text; add a reading item and choose a saved reading; add a song (sequence filled from its arrangement), reorder entries, set a part and a key change; assign a member and a free-text name; reload and everything is there; the library refuses to delete the song while the liturgy exists |
| E2E-W-014 | "Prepare next week": two services, one slot already existing; create; the list shows them |
| E2E-W-015 | Two sessions: both edit different items; both edit one item and the second sees the conflict with its text kept; the first sees the change after a reload; undo and redo with the buttons and with Ctrl+Z; undo refused after the other session's change |
| E2E-W-005 | axe on the list, prepare, new, editor (several items, with a conflict shown and a locked liturgy), and the planning pages; the editor also fits 320 px without sideways scrolling |

## 9. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Make Ctrl+Z always call the server | Only outside text fields | Text fields must keep their native undo |
| Undo "the latest edit of the liturgy" | The caller's own latest edit, refused if **another person's** row touched it since | SPEC §5.2: one person must not undo another's work |
| Compare version numbers to decide whether an undo is allowed | Compare history rows (`seq`) written by other people | The caller's own undos raise the versions, so a version test refuses the second undo |
| Hold a lock while someone edits | Versions and the conflict screen | Stale locks block a team on Sunday morning |
| Reorder with drag and drop only | Buttons with text (P-52) | Older users, keyboards, screen readers |
| Use colour only for "unsaved" or "locked" | Text as well | WCAG 1.4.1 |
| Skip a refused *structural* edit so that older ones can be undone | Skip only non-structural edits | A removed item comes back at the wrong place when a later add of the same person stays |
| Re-create an item with the version of its original `item.add` | The version it had when removed, plus 1 | Versions must never go back (stale tabs) |
| Let an undo reach back past a review round | `undo_floor_seq` | Undoing an edit a reviewer has read |

## 10. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| Item `version_conflict` | 409 `scope: "item"` | "This item was changed by someone else." | Show their version / keep mine |
| Liturgy `version_conflict` | 409 `scope: "liturgy"` | "The items were changed by someone else. The list was reloaded." | Repeat the action |
| `undo_refused` | 409, `reason` | `nothing_to_undo`: "Nothing to undo." `changed_since`: "Someone has changed this since, so it can't be undone." `reference_gone`: "Something this change used no longer exists." | — |
| `liturgy_locked` | 409 | "This liturgy can't be edited now." | Reload |
| `forbidden`, `not_found` | 403, 404 | As in [10 §12](10-liturgy.md#12-error-handling-matrix) | — |

## 11. References

| Topic | Location |
|---|---|
| Concurrent editing, undo | [SPEC.md §5.2](../SPEC.md#52-weekly-liturgy), [§11](../SPEC.md#11-decisions-log) (concurrent editing and per-person undo rows) |
| Data and API | [09](09-planning.md), [10](10-liturgy.md), [reference/schema.md](../reference/schema.md#step-3-tables) |
| Web shell, accessibility, translations | [05](05-web-shell.md) |
| Song and reading pickers | [06 §3](06-song-library.md#3-api), [07 §4](07-readings.md#4-api) |
