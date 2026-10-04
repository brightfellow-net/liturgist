# 12 — Review Workflow: States, Comments, State History (Implementation)

> **Document type: Implementation.** Step 4 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), in two slices: 4A states and history, 4B comments.
> Status: decisions **[P-xx] Approved** 2026-10-04 (after the Spec Gate and adversarial review round 4). Code may be written from this document. Items marked **[P-xx]** are decisions listed in the [index](README.md#4d-proposed-decisions-step-4). Nothing here is Approved until the owner says so.

## 1. Scope

The review states of [SPEC.md §5.5](../SPEC.md#55-review-workflow) up to **Approved**, the notes and history of state changes, and item-level comments with a resolve flag.

| Left out | Why | Where it lands |
|---|---|---|
| Approved → Published, `PublishedVersion`, reopening a published liturgy, archiving | Owner decision 2026-10-04: step 4 builds only up to Approved, so the freeze rules are written once, with the snapshot. Entitlement checks on reopen and unarchive ([10 §8](10-liturgy.md#8-entitlements-and-limits)) go with them | Step 5 |
| What a reopened liturgy with cleared references (P-61) looks like | Only published liturgies can hold cleared references ([10 §6](10-liturgy.md#6-usage-ports-and-deleted-songs-readings-sections)); step 4 never reopens one | Step 5 |
| Threaded replies, editing or deleting a comment | Owner decision 2026-10-04: flat comments with a resolve flag | Not planned |
| Notifications (WhatsApp, e-mail) about reviews | SPEC §5.6 covers WhatsApp for published liturgies only | After the MVP |
| Withdrawing a submission (In Review → Draft) | Not in SPEC §5.5; the reviewer sends it back | Not planned |

## 2. States and transitions [P-68]

`liturgies.state` already allows every value ([schema](../reference/schema.md#liturgies)). Transitions are the only way to change it; `PATCH /liturgies/{id}` never touches it.

| Action | From → to | Scope | Body |
|---|---|---|---|
| `submit` | `draft` or `needs_revision` → `in_review` | `liturgy.edit` | `{ note? }` |
| `approve` | `in_review` → `approved` | `liturgy.approve` | `{ edit_seq, note? }` |
| `request_changes` | `in_review` → `needs_revision` | `liturgy.approve` | `{ edit_seq, note? }` |
| `reopen` | `approved` → `draft` | `liturgy.approve` | `{ note? }` |

Rules:

- **No self-approval ban, no gate on open comments [owner decision 2026-10-04].** The person who submitted may approve if they hold `liturgy.approve`. Approving with unresolved comments succeeds. The page warns from the `open_comments` of the liturgy view it displays (asks once, inline); a comment added after the page loaded is simply not in the warning, and the number in the approve response is the one at commit time.
- **Submit is refused with problems [P-69].** A liturgy with no items → 422 `empty_liturgy`. A liturgy whose `problems` ([10 §2.5](10-liturgy.md#25-problems)) are not empty → 422 `has_problems` with the list. Nothing else blocks a submit, and `approve` does not re-check (nothing can change while the liturgy is in review, §2 "Edits cannot slip in"). **The check and the state change are one atomic step:** submit reads `state`, `edit_seq`, the item count and the problems in one transaction, and its update is conditional on both (below), so an edit committed after the read makes the update change no row (409 `review_stale`, "The liturgy changed while you submitted. Try again.") instead of letting an unchecked liturgy into review. Submit carries no `edit_seq` from the client: the submitter commits to what the server has at that moment.
- **`note`**: 0–500 characters after trimming, counted in runes, `\r\n` turned into `\n` (the same function as the comment body, §4); stored on the state change. Shown to the team in the history; for `request_changes` it is the reviewer's summary.
- **`edit_seq` on approve and request changes [P-70].** The page sends the `edit_seq` ([10 §5](10-liturgy.md#5-versions-and-conflicts-p-60)) it displayed. A mismatch → 409 `review_stale`, nothing changes. Without it, a reviewer's tab left open since the first round could approve content changed in a later round that they never read.
- **The state is claimed by a conditional update** `UPDATE liturgies SET state = $to, undo_floor_seq = edit_seq, updated_at = $now WHERE church_id = $1 AND id = $2 AND state = $from AND edit_seq = $seq RETURNING edit_seq`. `$from` is the state the use case read (for submit, either `draft` or `needs_revision`, so the recorded `from_state` is exact); `$seq` is the client's `edit_seq` for approve and request changes, and the one read in the same transaction for submit and reopen. The returned `edit_seq` goes into the state change row.
- **Order of checks [P-68]:** 404 (not visible) → 403 (scope) → 422 (`validation_failed`: note too long, `edit_seq` missing where required) → 409 `invalid_transition` (the state read is not a source of the action) → 422 `empty_liturgy` / `has_problems` (submit) → the update. For approve and request changes a stale `edit_seq` is checked **inside** the update. When the update changes no row, the use case re-reads: liturgy gone → 404; state differs from the one read → 409 `invalid_transition`; otherwise → 409 `review_stale`. So the state wins when both apply. Two simultaneous transitions: exactly one wins.
- **Every transition sets `undo_floor_seq` to `edit_seq`** in the same statement ([11 §7.2](11-liturgy-editor.md#72-undo-and-redo-p-65)), so nobody undoes an edit a reviewer has read. A transition writes no `liturgy_edits` row and does not change `edit_seq` or `version`.
- **Edits cannot slip in after a transition [P-71].** Every history-writing statement of [10 §5](10-liturgy.md#5-versions-and-conflicts-p-60) (`NextSeq`) becomes `UPDATE liturgies SET edit_seq = edit_seq + 1 WHERE id = $1 AND church_id = $2 AND state IN ('draft','needs_revision') RETURNING edit_seq`. Zero rows means "gone or locked", which one statement cannot tell apart, and today `NextSeq` maps zero rows to `not_found` (`adapters/sqlstore/liturgies.go`) while `record` and `undo.step` pass that on, so a lost race would answer 404 or 500. **Required change [P-71]:** the repository returns a new sentinel `app.ErrNoSeq` on zero rows; one helper `nextSeq(ctx, sc, id)` in `app/liturgy_history.go`, used by `record` and by `undo.step`, then re-reads the liturgy: gone → `not_found`, else `ErrLiturgyLocked` (409 `liturgy_locked`); the transaction rolls back. The state check at the start of a write ([10 §5](10-liturgy.md#5-versions-and-conflicts-p-60) step 1) stays as the early, cheap refusal; this is the one that cannot race, because the update waits for the transition's row lock and re-reads the state (PostgreSQL `READ COMMITTED`; SQLite has one writer). Undo and redo take the same statement. The later bookkeeping writes of undo (`MarkSkipped`, `DropUndone`, [11 §7.2](11-liturgy-editor.md#72-undo-and-redo-p-65)) change status flags only, do not use `NextSeq` and are exempt; an undo or redo that meets a locked liturgy answers `liturgy_locked`, never `changed_since`.
- **Locks:** a transition takes no `LockChurch` (it counts nothing). Its only write lock is the liturgy row lock, taken by the update; it holds no other lock while it waits, so it cannot be part of a lock cycle with an edit (item row, then liturgy row), an undo (church lock, then liturgy row), a delete or a comment (liturgy row only). A delete that wins makes the transition's update change no row and the re-read answer 404. Tests: IT-R-004 and IT-R-011.
- **Delete** stays for `liturgy.manage` in the four unpublished states ([10 §2.1](10-liturgy.md#21-liturgy)) and removes the comments and state changes with the liturgy (`ON DELETE CASCADE`).
- **Visibility and `actions`:** the liturgy view's `actions` gains `submit`, `approve`, `request_changes`, `reopen` and `comment`, each true when the member holds the scope **and** the state allows it ([04 §5](04-tenancy-extensions.md#5-authorization)); the view also carries `edit_seq`, `open_comments` (the number of unresolved comments) and `last_change` (`{ to_state, user: { id, name }, note, created_at }` of the newest state change, or null), which feed the banners and the warning, so no extra request is needed. No lookups for `actions`; the two counts come from the same read as the liturgy.
- **`reopen` is reserved [P-68]:** in step 5 the same route also reopens a `published` liturgy (with the entitlement check of [10 §8](10-liturgy.md#8-entitlements-and-limits)); step 4 allows only `approved` → `draft`.
- **Who sees the review data [P-74].** The comment and state-change routes and the `open_comments` and `last_change` fields need a `liturgy.*` scope (`canSeeLiturgies`) **whatever the state**: unlike the liturgy itself, they are never shown to a member without one, not even for a `published` liturgy (step 5). 404 otherwise. Do not reuse `openLiturgy` for them; add `openReviewable`.

## 3. API [P-68]

All routes are under the church path, need a session and the CSRF header, and follow the 401/404/403 rules of [10 §4](10-liturgy.md#4-editing-items-songs-and-assignments).

| Method & path | Request | Response |
|---|---|---|
| `POST /liturgies/{id}/submit` | `{ note? }` | 200 the liturgy view |
| `POST /liturgies/{id}/approve` | `{ edit_seq, note? }` | 200 the liturgy view; `open_comments` is the number at commit time (informational) |
| `POST /liturgies/{id}/request-changes` | `{ edit_seq, note? }` | 200 the liturgy view |
| `POST /liturgies/{id}/reopen` | `{ note? }` | 200 the liturgy view |
| `GET /liturgies/{id}/state-changes?limit=&offset=` | — | `{ items: [{ id, from_state, to_state, user: { id, name }, note, edit_seq, created_at }], total }`, newest first; `limit` default 50, at most 200 |
| `GET /liturgies/{id}/comments?resolved=` | — | `{ items: [Comment], open: n }`, oldest first; `resolved=true/false` filters |
| `POST /liturgies/{id}/comments` | `{ item_id?, body }` | 201 the comment |
| `PUT /liturgies/{id}/comments/{cid}/resolved` | `{ resolved }` | 200 the comment |

`Comment` = `{ id, item_id?, item_title?, author: { id, name }, body, resolved, resolved_by?: { id, name }, resolved_at?, created_at }`. `author.name` is `users.name` (a join on the user, not on the membership), so it stays after the person left the church; `item_title` is the title kept when the comment was written (§5).

## 4. Comments [P-72]

- **Who and when.** Creating and resolving need `liturgy.comment`; the liturgy must be in `draft`, `in_review` or `needs_revision` (409 `liturgy_locked` in `approved` and `published`, like every other write). Reading needs a `liturgy.*` scope (§2, P-74). Order of checks (comments differ from transitions on purpose: a comment has no state-dependent validation): 404 → 403 → 409 `liturgy_locked` → validation (422) → the write. A comment id is always looked up with `church_id`, `liturgy_id` **and** `id`; one of another liturgy or church → 404. Anyone with `liturgy.comment` may resolve or reopen any comment, not only their own: the editor who fixed the point marks it done.
- **Fields.** `body` 1–2,000 characters (trimmed; `\n` line endings; plain text, never rendered as HTML). `item_id` optional: a null item means the liturgy as a whole. An `item_id` that is not an item of this liturgy → 422. At most **500 comments per liturgy**, resolved ones included (422 `comment_limit`): the cap is a plain ceiling that a real liturgy does not reach, and comments are never deleted.
- **Resolved** is toggled with `PUT …/resolved`; it records who and when. Setting it to the value it already has is a no-op (200); resolving is `UPDATE … WHERE resolved_at IS NULL`, so with two simultaneous resolves the first is recorded. The state check comes before the no-op.
- **No edit, no delete.** A comment written in error is resolved. Deleting the liturgy removes its comments.
- **Removed items [P-72].** A comment is kept when its item is removed (a reviewer's words must not vanish with an edit), so `item_id` has **no foreign key**; the app checks it on create. The comment keeps `item_title` as of writing and the page shows it under "On a removed item: …". Restoring the item by undo ([11 §7.2](11-liturgy-editor.md#72-undo-and-redo-p-65)) brings the same ID back, so its comments attach again. Moving or retitling an item does not touch its comments.
- **Counting.** `open` is always the total number of unresolved comments of the liturgy, whatever the `resolved` filter; the editor shows it per item (counted from the list) and in total.
- **Races [P-74].** Creating and resolving start with `UPDATE liturgies SET updated_at = updated_at WHERE church_id = $1 AND id = $2 AND state IN ('draft','in_review','needs_revision')`: a no-op that takes the liturgy row lock and, on PostgreSQL, re-reads the state after waiting for a transition. Zero rows → re-read: gone 404, else `liturgy_locked`. Under that lock the transaction counts the comments (the 500 cap), checks the item, **reads the item's title** and inserts, so the limit cannot be passed by two simultaneous requests, `item_title` is the title the item had when the comment was accepted, and a comment is never added to a liturgy after the approval that took the same lock earlier (an approve that waits for the comment sees it in `open_comments`). The lock is held for the few statements only and is never taken together with another liturgy row or `LockChurch`. A liturgy deleted in the meantime makes the foreign key fail; the repository maps that to `not_found`.
- **Logging:** `comment_added`, `comment_resolved`, `comment_reopened` with IDs and actor; never the body.

## 5. Data model [P-73]

Tables in [reference/schema.md](../reference/schema.md#step-4-tables). Migrations are forward-only (P-11), so each slice has its own, in both dialect folders: `00008_state_changes.sql` (4A) and `00009_comments.sql` (4B).

| Table | Purpose |
|---|---|
| `liturgy_state_changes` (4A) | `id`, `church_id`, `liturgy_id` (cascade), `from_state`, `to_state` (both with the state check), `user_id` (→ `users`, RESTRICT, as `liturgy_edits.user_id`; like it, this makes erasing a user account a matter for a later decision, not for this step), `note` (0–500, empty when none), `edit_seq` (the liturgy's at that moment), `created_at`. Index (`church_id`, `liturgy_id`, `created_at`, `id`); `UNIQUE liturgy_state_changes_church_id_key (church_id, id)` |
| `liturgy_comments` (4B) | `id`, `church_id`, `liturgy_id` (cascade), `item_id` (nullable, **no foreign key**), `item_title` (snapshot, empty for the whole liturgy), `author_id` (→ `users`, RESTRICT), `body` (1–2000), `resolved_at`, `resolved_by` (both null or both set; → `users`, RESTRICT), `created_at`. Index (`church_id`, `liturgy_id`, `created_at`, `id`); `UNIQUE liturgy_comments_church_id_key (church_id, id)`; partial index on unresolved rows (schema) |

No change to `liturgies`: `state` and `undo_floor_seq` exist.

## 6. Pages (slice 4B for comments; the review bar belongs to 4A)

All text through i18n ([05 §6](05-web-shell.md#6-translations)); the pages work at 320 px width and 200% text. No modal dialogs: a note is entered inline, as in `ConfirmButton`.

| Place | What |
|---|---|
| Liturgy page header | The state in words and the buttons the `actions` allow ("Submit for review", "Approve", "Request changes", "Reopen"). Approve and request changes ask for an optional note inline; approve first shows "N comments are still open." with "Approve anyway". A refused submit lists the problems with links to the items |
| Banner | In `in_review`: "This liturgy is being reviewed. Only comments can be added." In `approved`: "Approved. Reopen it to make changes." In `needs_revision`: the note of `last_change` |
| Item card | A comment count and a "Comment" button for `actions.comment`; the item's comments below it, with a "Resolve" / "Reopen" toggle; resolved ones are collapsed |
| Review panel (tab beside History) | All comments, "Open" and "Resolved" filters, a form for a comment on the whole liturgy; below it the state history (who, when, from → to, note) |
| `/liturgies` list | Unchanged; the state words cover the new states |

Keyboard and screen readers: buttons have text, the state is announced through a live region when it changes, each comment is a list item with the author and time in text.

## 7. Slices

| Slice | Content |
|---|---|
| 4A | Migration `00008_state_changes.sql`, transitions, `edit_seq`, `open_comments` (0 until 4B), `last_change` and `actions` in the view, the conditional `NextSeq` with `ErrNoSeq` and the `nextSeq` helper in `record` and `undo.step`, state history route, review bar and banners |
| 4B | Migration `00009_comments.sql`, comment routes, `openReviewable`, item cards, review panel |

Each slice is drafted, shown and approved on its own, as in step 3.

### 7.1 Slice 4A as built (2026-10-04)

Merged as `9f8cbe7`. Differences and additions to the text above:

- **Bodies.** The four transition routes take a JSON body; a request with no body is refused (400), so `submit` and `reopen` need `{}`. The web client sends it. Making the body optional is not possible with the Huma version in use.
- **Liturgy view.** `open_comments` is present (0) for a member with a `liturgy.*` scope until slice 4B; `last_change` is **omitted**, not null, when the liturgy never changed state; both are omitted for a member without a scope. The `comment` action arrives with 4B.
- **Code.** `domain/review.go` (the transition table, `ValidateNote`), `app/liturgy_review.go` (`Review`, `StateChanges`, `openReviewable`), `app/liturgy_history.go` (`nextSeq`), `LiturgyRepo.Transition`, `StateChangeRepo`, `adapters/sqlstore/state_changes.go`, `web/src/routes/liturgy/Review.tsx`. `sqlstore` test hooks gain `DialectName` and `CountForTest`.
- **Tests.** TC-R-001, 003, 006, 007 and the `CanComment` rule; IT-R-001 to 006, 009 to 012 (the free-running race `TestEditAndSubmitRace` runs 60 rounds), the two forced PostgreSQL interleavings (`TestEditLosesToTransition`, `TestSubmitLosesToEdit`, skipped on SQLite, which has one writer; removing the `state IN` predicate or the `edit_seq` condition makes them fail), WT-R-001 to 003 and 005, E2E-W-016. IT-R-007, 008, 013, 014 and WT-R-004 wait for 4B. Vitest 211, Playwright 36.
- **Not run:** golangci-lint (the v1/v2 config mismatch of step 3 still stands).

### 7.2 Slice 4B as built (2026-10-04)

Merged as `5831ed7`. Step 4 is complete. Differences and additions:

- **Migration** `00009_comments.sql` (both dialects); `item_id` has no foreign key, as written.
- **Routes** as in §3 (`listLiturgyComments`, `addLiturgyComment`, `setLiturgyCommentResolved`); the liturgy view's `open_comments` is the real count and `actions.comment` exists (scope `liturgy.comment` and a state of `draft`, `in_review` or `needs_revision`). The approve response counts the comments committed before it.
- **Code.** `app/liturgy_comments.go`; `LiturgyRepo.LockForComment` (the no-op update of §4); `CommentRepo` (`Create`, `List`, `Count`, `Open`, `ByID`, `SetResolved`); `domain.Comment`, `ValidateComment`; `web/src/routes/liturgy/Comments.tsx`. The use case reads the clock before taking the lock, so a test can hook the interleaving there.
- **Web.** The item's comments sit under its card (group "Comments on {title}", form "Comment on {title}"); the review panel lists all comments with the filters Open (the start), Resolved and All, and holds the form for the whole liturgy. The in-review banner now says that only comments can be added.
- **Tests.** TC-R-005 (domain and app), IT-R-007, 008, 009, 013, 014 and the comment rows of IT-R-010, the 500-comment cap with 20 simultaneous requests at the edge, `TestCommentAndApproveRace` (40 rounds, both dialects: an accepted comment is always counted by the approval), the forced PostgreSQL interleaving `TestCommentLosesToApproval` (skipped on SQLite; removing the state test from `LockForComment` makes both race tests fail), WT-R-004, and E2E-W-016 extended with a comment, a resolve and axe on the page with comments. Vitest 217, Playwright 36.
- **Not run:** golangci-lint (the v1/v2 config mismatch of step 3 still stands).

## 8. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Change `state` through `PATCH /liturgies/{id}` | Only the four transition routes | Each transition has its own scope, note and history row |
| Check the state with a read and then write | Conditional update on `state` and, for edits, on `NextSeq` | A transition between the read and the write would let an edit into a locked liturgy |
| Approve without the `edit_seq` the reviewer saw | Require it; 409 `review_stale` | A stale tab would approve unread content |
| Ban self-approval or block on open comments | Allow both; warn on open comments | Small churches have one liturgist; the owner decided 2026-10-04 |
| Delete a comment when its item is removed | Keep it with `item_title`; undo re-attaches it | Reviewers' words must survive edits |
| Let `request_changes` or `reopen` leave `undo_floor_seq` alone | Every transition sets the floor | Undo must not reach back past a review round |
| Render a comment body as HTML or Markdown | Plain text, escaped | Comments are the only text other members write that the author must read safely |
| Reuse `openLiturgy` for comments or history | `openReviewable` (any `liturgy.*` scope, whatever the state) | A published liturgy is visible to every member; its reviewers' words are not |
| Check the submit conditions, then update by state only | Update on `state` and `edit_seq` read with the check | An edit between check and update would put an unchecked liturgy into review |
| Log comment bodies or notes | IDs and actor only | Privacy |

## 9. Test case specifications

### Unit tests (domain)
| ID | Component | Input | Expected |
|---|---|---|---|
| TC-R-001 | Transition table | every (state, action) pair | exactly the four rows of §2 are allowed; each other pair → `invalid_transition` |
| TC-R-002 | Scopes | each action without its scope | 403 `forbidden` before any state check |
| TC-R-003 | Problems gate | empty items; each of the five problem codes (`song_missing` and `reading_missing` through the API; the three `*_removed` codes need rows set up directly by a fixture helper, because step 4 cannot create them) | `empty_liturgy`; `has_problems` listing each |
| TC-R-004 | `Actions` | state × scope matrix | `submit`, `approve`, `request_changes`, `reopen`, `comment`, `edit` as §2 and §4 say |
| TC-R-005 | Comment validation | blank, 2,001 characters, foreign `item_id`, 501st comment | 422 each; `item_id` of this liturgy accepted |
| TC-R-006 | Text rules | note and body with spaces, `\r\n`, multi-byte characters at the limit | trimmed, `\n`, counted in runes; 500 / 2,000 accepted, one more refused |
| TC-R-007 | Order of checks | invisible + forbidden + invalid + stale combinations | 404, 403, 422, 409 `invalid_transition`, 409 `review_stale` in that order; the state wins over a stale `edit_seq` |

### Integration tests (HTTP, both dialects)
| ID | Flow | Verification |
|---|---|---|
| IT-R-001 | Draft → submit → request changes → edit → resubmit → approve → reopen | States and the history rows (user, note, `edit_seq`) are exact; edits succeed only in `draft`/`needs_revision` |
| IT-R-002 | Approve with a stale `edit_seq` (an edit and a resubmit in between) | 409 `review_stale`; state unchanged |
| IT-R-003 | Two simultaneous approve / request-changes | Exactly one 200, the other 409 `invalid_transition` |
| IT-R-004 | Edit and transition forced into both interleavings with test hooks (`sqlstore/testhooks.go`): the edit passes its early state check, then the transition commits; and the reverse. Then 100 free-running rounds on both dialects as a smoke test | Forced: the edit returns 409 `liturgy_locked` (**not** 404 or 500), writes no row, and an undo meeting the lock answers `liturgy_locked`. Invariant at the moment of every transition: its `undo_floor_seq` equals the newest `liturgy_edits.seq`, and no `liturgy_edits` row has a `seq` above the floor while the state is locked |
| IT-R-011 | Submit racing an edit that adds a problem or removes the last item (hook between the check and the update) | 409 `review_stale`; the liturgy stays in `draft`; delete racing a transition → 404 |
| IT-R-012 | A member with no `liturgy.*` scope and a `published` liturgy (fixture set by SQL; publishing is step 5) | 404 on comments and state changes; `open_comments` and `last_change` absent from the liturgy view |
| IT-R-013 | Comment racing an approve, and 501 simultaneous comments | Approve's `open_comments` counts every comment committed before it; no comment exists on the `approved` liturgy after the approval; at most 500 comments exist |
| IT-R-014 | Resolve with a comment id of another liturgy or church; two simultaneous resolves | 404; the first resolver is recorded |
| IT-R-005 | Undo across a transition | An edit before submit is no undo target after resubmit (`nothing_to_undo`); one made after is |
| IT-R-006 | Approve with open comments | 200 with `open_comments` = n; self-approval by the submitter works |
| IT-R-007 | Comments: create, resolve, reopen, list filters; remove the item; undo the removal | Comment kept with `item_title`; re-attached after the undo |
| IT-R-008 | Comment in `approved`, and in `published` (state set by SQL, as step 4 cannot publish) | 409 `liturgy_locked`; resolve too |
| IT-R-009 | Permissions across churches and scopes | A member of church A never sees church B's comments or history (404); `liturgy.comment` without `liturgy.edit` can comment but not edit; a team member with no `liturgy.*` scope gets 404 |
| IT-R-010 | Delete an unpublished liturgy | Comments and state changes are gone with it |

### Web (Vitest, `mockApi` and `renderPage`)
| ID | Subject | Verification |
|---|---|---|
| WT-R-001 | Review bar | Shows exactly the buttons `actions` allows in each state; hides all of them without scopes; the banner text per state; the `needs_revision` banner shows the `last_change` note |
| WT-R-002 | Approve | With `open_comments` > 0 asks once ("Approve anyway"), then sends `edit_seq`; `review_stale` shows the reload message and keeps the page usable |
| WT-R-003 | Submit | `has_problems` lists the problems with links to the items; `empty_liturgy` message |
| WT-R-004 | Comments | Add on an item and on the liturgy, resolve and reopen, resolved ones collapsed, a removed item's comment shows its kept title, the editor stays read-only in `in_review` while comments work |
| WT-R-005 | i18n and accessibility | Every string through `review.*` keys in `en` and `id` (Indonesian is a draft for the owner's review); the state change is announced in a live region |

### End-to-end (Playwright)
E2E-W-016: an editor submits, the liturgist adds an item comment, requests changes, the editor resolves it and resubmits, the liturgist approves and reopens; axe passes on the page in each state.

## 10. Error handling matrix

| Error | Code | Detection | User message (en) | Recovery |
|---|---|---|---|---|
| Wrong state for the action | 409 `invalid_transition` (`state`) | Conditional update changed no row | "This liturgy is now {{state}}." | Reload |
| Reviewed content changed | 409 `review_stale` | `edit_seq` differs | "The liturgy changed after you opened it. Read it again." | Reload |
| No items | 422 `empty_liturgy` | Item count 0 | "Add at least one item before submitting." | — |
| Unfinished items | 422 `has_problems` (`problems`) | §2 | "Some items are not finished." with the list | Go to the item |
| Locked | 409 `liturgy_locked` | State not allowed | "This liturgy can't be edited now." | Reload |
| Missing or too long input | 422 `validation_failed` | `edit_seq` missing (approve, request changes), note over 500 | "Check this field." | Fix and resubmit |
| Too many comments | 422 `comment_limit` | 500 reached | "This liturgy has reached its limit of 500 comments." | Continue in a new liturgy or note |
| Item not in this liturgy | 422 `validation_failed` | §4 | "Check this field." | — |
| No permission | 403 `forbidden` | Missing scope | "You don't have permission to do this." | — |
| Not visible / gone | 404 `not_found` | — | "This liturgy no longer exists." | Back to the list |

## 11. References

| Topic | Location |
|---|---|
| Review workflow | [SPEC.md §5.5](../SPEC.md#55-review-workflow), [§7](../SPEC.md#7-data-model-sketch), [§11](../SPEC.md#11-decisions-log) |
| Liturgy states, editing, versions, `problems`, history | [10 §2](10-liturgy.md#2-data-model), [§4](10-liturgy.md#4-editing-items-songs-and-assignments), [§5](10-liturgy.md#5-versions-and-conflicts-p-60), [§7](10-liturgy.md#7-history) |
| Undo floor | [11 §7.2](11-liturgy-editor.md#72-undo-and-redo-p-65) |
| Authorization and `actions` | [04 §5](04-tenancy-extensions.md#5-authorization) |
| Tables | [reference/schema.md](../reference/schema.md#step-4-tables) |
