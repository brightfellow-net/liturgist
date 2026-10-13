# 16 — Refactoring before the first public release (Implementation)

> **Document type: Implementation.** Behaviour-preserving clean-up of the longest functions, and a few publication-readiness items, before `v0.1.0` is tagged and the module is requested on pkg.go.dev.
> Status: decisions **[R-1] to [R-11] Approved** 2026-10-13, as proposed (R-9 to R-11 were added the same day, after the question of reusing `domain` and `app` for the hosted edition). Spec Gate and adversarial review have not run; slices are drafted one at a time and each waits for the owner's approval before merging. Nothing is changed yet.

## 1. What was measured

On 2026-10-13, `main` at `024fa06`, Go 1.27, with `golangci-lint` v2.14.0 and extra linters switched on for the survey only (thresholds below; the project config is unchanged):

| Check | Result |
|---|---|
| Duplicate code (`dupl`, 120 tokens) | None |
| `TODO`, `FIXME`, `HACK` markers | None |
| Cyclomatic complexity over 25 (`gocyclo`) | 3 functions: `httpapi.MapError` 36, `cmd/liturgist.run` 32, `server.Restore` 29 |
| Cognitive complexity over 35 (`gocognit`) | 19 functions, listed in §2 |
| Function length over 120 lines (`funlen`) | 3 functions: `RegisterLibrary` 151, `registerImports` 144, `registerReadings` 137 |
| Deep nesting (`nestif` over 8) | 3 places: `paste.Build`, `Songs.updateIn` (sections), `backup.Write` |
| Unused parameters (`unparam`) | 2: `paste.Build` (an unused result), `domain.validateName` (`field` is always `"name"`) |
| Largest Go files | `app/imports.go` 753, `sqlstore/liturgies.go` 733, `httpapi/liturgy_ops.go` 631, `app/ports.go` 623 |
| Largest web files | `MembersPage.tsx` 321, then under 300 |
| Packages without a package comment | None. (The survey first reported `cmd/liturgist`; it has a "Command liturgist …" comment, and the check had looked only for the "Package" form.) |
| Statement coverage, SQLite only, across packages | 86.1 %; the functions in §2 are 72 % to 99 % |

The code is in good shape. The candidates are a short list of long functions, most of them *registration* functions (a flat list of operations) or *parsers*.

## 2. Candidates

| Group | Function | Cognitive | Covered |
|---|---|---|---|
| **A. HTTP registration** | `httpapi.RegisterLiturgies` | 101 | 97 % |
| | `httpapi.RegisterChurch` | 66 | 95 % |
| | `httpapi.RegisterPlanning` | 45 | 88 % |
| | `RegisterLibrary`, `registerImports`, `registerReadings` (length, not complexity) | | 93–98 % |
| **B. Importers** | `openlyrics.Importer.Parse` | 96 | 98 % |
| | `chordpro.parseSong` | 70 | 91 % |
| | `paste.Build` | 58 | 99 % |
| **C. Use cases** | `buildPublished` | 67 | 85 % |
| | `Liturgies.Review` | 61 | 85 % |
| | `Liturgies.PrepareCreateArchiving` | 56 | 82 % |
| | `Liturgies.SetSongs` | 53 | 90 % |
| | `Imports.Create` | 52 | 93 % |
| | `Liturgies.UpdateItem`, `Liturgies.step` (undo) | 50 | 90 %, 86 % |
| | `Liturgies.AddAssignment` | 48 | **80 %** |
| | `Imports.Decide`, `Setup.Run`, `Auth.Login`, `songsOf`, `domain.matchSections` | 36–46 | 71–100 % |
| **D. Entry points** | `cmd/liturgist.run` (a dispatch `switch`) | cyclomatic 32 | **47 %** in-process |
| | `httpapi.MapError` (a flat `switch`) | cyclomatic 36 | 88 % |
| | `server.Restore` (staged, already commented) | cyclomatic 29 | 82 % |

## 3. Decisions

| ID | Proposal | Status |
|---|---|---|
| R-1 | **Rule: nothing observable changes.** No new feature, no changed behaviour, no changed exported signature, no changed transaction boundary or lock order, no schema change. The private repository `liturgist-saas` imports `domain`, `app` and `adapters`, so exported names stay as they are. A function may be split, a helper may be added; an exported function may not be renamed or lose a parameter | **Approved** 2026-10-13 |
| R-2 | **Slice A first (HTTP registration).** Split each long `Register*` function into functions per operation group (`registerLiturgyEdit`, `registerLiturgyReview` …) in the same file, each under about 60 lines. The guard is strong and cheap: after the change `make gen` must give a byte-identical `packages/api-client/openapi.json` and `schema.d.ts` (`git diff --exit-code`), plus the whole HTTP suite and `TestTenancyDeclarations` | **Approved** 2026-10-13 |
| R-3 | **Slice B (importers).** Split `Parse`, `parseSong` and `Build` into stages (read, build sections, derive title and key) with plain data between them. Guard: the existing importer tests with their sample files, which cover 91–99 % | **Approved** 2026-10-13 |
| R-4 | **Slice C (use cases), in three slices:** C1 publishing and review (`buildPublished`, `Review`, `songsOf`, `step`); C2 editing (`UpdateItem`, `SetSongs`, `AddAssignment`, `PrepareCreateArchiving`); C3 import, setup and login (`Imports.Create`, `Decide`, `Setup.Run`, `Auth.Login`). **Before moving code, every function below 85 % covered gets characterization tests first** (`AddAssignment`, `PrepareCreateArchiving`, `songsOf`, `Setup.Run`, `Review`, `buildPublished`, `Login`), in its own commit so the old behaviour is pinned. Each extracted helper keeps working inside the same transaction closure; the call to `actorIn(..., lock)` stays first | **Approved** 2026-10-13 |
| R-5 | **Slice D, only part of it.** Do `cmd/liturgist.run` (a table of commands; adds in-process tests, since it is 47 % covered). **Leave `MapError`** (a flat table of cases is clear as it is) **and `server.Restore`** (staged and commented; splitting it moves risk into the one command that must never fail) | **Approved** 2026-10-13 |
| R-6 | **Tidy slice.** Remove the two `unparam` findings. (The package comment of `cmd/liturgist` named here exists already; see §1) | **Approved** 2026-10-13 |
| R-7 | **Publication readiness (no code moves).** (a) State in `README.md` and in the package comments of `domain`, `app` and `server` that the Go API is **pre-1.0 with no stability promise**; (b) say that `go install` is not an install method (the web app is not in git; use Docker or a release); (c) list which packages are meant to be imported (`domain`, `app`, `server`, `adapters/*`) and that `internal/` is not; (d) tag `v0.1.0-rc1` after the slices, then `v0.1.0` | **Approved** 2026-10-13 |
| R-8 | **No new lint rule now.** After slice C the survey is run again and its numbers go into §6; whether to switch `gocognit` on at a lenient limit (50) in `.golangci.yml` is the owner's call then | **Approved** 2026-10-13 |
| R-9 | **A second edition's contract.** Add a short section to `README.md` and to the package comment of `server` that says what the hosted edition (`liturgist-saas`) may depend on: **the composition seam** (`server.New` with the options `WithRoutes`, `WithTenantResolver`, `WithEntitlements`, `WithURLBuilder`, `WithStorage`, `WithEventBus`, `WithNotifier`, `WithBibleTextProvider` and `WithImporter`, and the exported `httpapi.Register*` functions) and the **extension ports** behind those options (`Entitlements`, `URLBuilder`, `Storage`, `EventBus`, `Notifier`, `BibleTextProvider`, `Importer`, `httpapi.TenantResolver`). `AuthProvider` and `Exporter` are declared in `app/extensions.go` but no option wires them (nothing calls them yet), so they are **not** part of the contract until one does. Everything else in `app` and `domain` is reachable but not part of that contract. Rules for the ports: keep each as small as possible; add a new capability as a **new optional interface** that the caller checks for, not as a new method on an existing one; every change to a port is listed under **Ports** in `CHANGELOG.md` (04 §7). Add a CI job to `liturgist-saas` (outside this repository) that builds it against core `main` every night, so a break is seen on the day it happens | **Approved** 2026-10-13 |
| R-10 | **Stored JSON rule.** The JSON tags in `domain` and `app` define persisted formats (church settings, published copies, undo history, import candidates). From now on every **new** stored JSON object (a) carries a `format` or `version` field unless it is a flat settings bag read by a tolerant reader, and (b) is read so that unknown keys are kept and a value of the wrong type reads as not set (the way `ChurchSettings` is read). Existing objects are not changed. The rule is written into 02 (persistence), where migrations are described | **Approved** 2026-10-13 |
| R-11 | **Decide before the first hosted signup, not before `v0.1.0`:** (a) a small **outbound `Mailer` port** (or an "invite created" and "reset requested" event) so the hosted edition can send invites and reset links by e-mail; today `EmailConfigured` is always false and the links are handed to an admin; (b) whether **church delete, per-church export and account deletion** are use cases in the core (so the community edition gets "export my church" too) or live only in `liturgist-saas` against `app.Store`; (c) **PostgreSQL row-level security** as a second wall for tenant isolation (an adapter change, no use-case change); (d) an **expand-then-contract migration rule** for rolling deploys, in 02, because the startup guard refuses a newer schema. This slice only records the four questions with a recommendation for each; nothing is built or approved by this document | **Approved** 2026-10-13, as a list of open questions; each of (a) to (d) is still decided on its own before the first hosted signup |

**Left out, and why**

| Not done | Reason |
|---|---|
| Splitting `app/ports.go` (623 lines) | It is the list of contracts; one file is easier to read than five. Cosmetic |
| Moving packages under `internal/` | Breaks `liturgist-saas`; the module is pre-1.0 anyway ([R-7]) |
| Splitting large test files (`liturgies_test.go` 990 lines) | Tests read top to bottom; no problem found |
| Web components | The largest is 321 lines; no function is complex enough to name |
| Replacing chi, Huma or sqlx | No reason found in the survey |

## 4. Slices and order

| Slice | Contents | Guard |
|---|---|---|
| 16A | R-2 HTTP registration, R-6 tidy | Byte-identical OpenAPI; HTTP suite; lint |
| 16B | R-3 importers | Importer tests; mutation check on the moved parsing rules |
| 16C1–C3 | R-4 use cases, characterization tests first | Full suite on SQLite and PostgreSQL, `-race`, the undo model test, mutation checks |
| 16D | R-5 `run` | New in-process tests; lint |
| 16E | R-7 readiness text, R-9 the second edition's contract, R-10 the stored-JSON rule; survey again; tag | Docs only |
| After `v0.1.0` | R-11 the four hosted-edition questions, each answered by the owner before the first hosted signup | A decision for each; no code in this document |

Each slice is drafted in its own worktree and waits for the owner's approval, as before. A slice that finds a bug does **not** fix it inside the refactoring: it adds a failing test, reports it, and the fix is its own change.

## 5. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Change behaviour "while there" | Report it and fix it in its own commit with its own test | A refactoring commit must be reviewable as "nothing changed" |
| Move code that is under 85 % covered | Pin it with characterization tests first | There is no net under it |
| Reorder reads and writes inside a transaction | Keep the order, including `actorIn` and the lock first | Lock order is the guard against races (03 §8) |
| Rename an exported name | Keep it, add a helper | `liturgist-saas` imports it |
| Regenerate the API client and accept the diff | Require a byte-identical file | Any difference is a behaviour change |
| Extract helpers that take ten parameters | Pass a small struct, or leave the code | A long parameter list is no clearer than a long function |
| Judge success by a lint score | Judge by readability of the diff and by the guards | The numbers are a survey, not a target ([R-8]) |

## 6. Verification

| Check | When |
|---|---|
| `go test -race ./...` with `LITURGIST_TEST_POSTGRES=1`; `golangci-lint run` | Every slice |
| `make gen` then `git diff --exit-code packages/api-client` | 16A, and every slice that touches `httpapi` |
| Mutation checks on the moved logic (drop a check, swap an order) | 16B, 16C |
| Coverage of the touched functions does not fall | Every slice (compare `go tool cover -func`) |
| The survey of §1 again, numbers added to this section | End of 16C, 16E |

## 7. References

| Topic | Location |
|---|---|
| Layering and import rules | [01 §3](01-foundation.md#3-repository-layout) |
| Lock order and the actor check | [03 §8](03-identity-auth.md#8-member-roles-and-permissions) |
| Lint set and the CI pins | [01-foundation.md](01-foundation.md), `.golangci.yml` |
| Release workflow and checklist | [14 §7](14-self-host.md#7-releases-slice-6d-h-10-to-h-13) |
| The pkg.go.dev question | the conversation of 2026-10-13: the module is public, untagged, not yet requested |
| Extension points and the stability promise to `liturgist-saas` | [04 §7](04-tenancy-extensions.md#7-extension-points) |
| SaaS hosting, tenancy and editions | [SPEC §8.1.1 and §8.2](../SPEC.md#811-saas-multi-church-hosting-moves-to-liturgist-saas) |
| Entitlements and limits | [04 §8](04-tenancy-extensions.md#8-entitlements) |

## 8. Slice 16A as built (2026-10-13)

| Part | Where |
|---|---|
| Each long `Register…` function is a list of calls to group functions; the handler code moved unchanged. `RegisterLiturgies`: week, open, change, item edits, item songs, assignments, undo, review, history. `RegisterChurch`: setup, church settings, members, roles, invites, password reset. `RegisterPlanning`: name lists, templates, services. `RegisterLibrary`: readings, imports, songs, song links. `registerImports`: batches, candidates, finish. `registerReadings`: lookup, browse, write | `adapters/httpapi/*_ops.go` |
| Shared helpers `failer`, `sessionOf`, `actorOf`, `churchOp` replace five copies of the same closures; `liturgyPath` and `readingResult` moved to package level | `adapters/httpapi/register.go`, `liturgy_ops.go`, `readings_ops.go` |
| Two `unparam` findings: the unused result of `paste.Build`'s `add`, and the `field` parameter of `domain.validateName` (always `"name"`) | `adapters/importers/paste/build.go`, `domain/planning.go`, `domain/liturgy.go` |

**Deviations from the plan**
- Groups are 40 to 100 lines, not "about 60": one handler is 20 to 30 lines. The longest function in `httpapi` is 103 lines (it was 151).
- `removeAssignment` moved into the assignments group (it was registered last). Registration order is not observable through the API; the byte-identical check covers it.
- The package comment of `cmd/liturgist` was not missing (see §1).

**Guards:** `make gen` gave a byte-identical `openapi.json` and `schema.d.ts`; 102 operations before and after; `TestTenancyDeclarations` unchanged; uncovered statements in `httpapi` 93 before and 93 after (merged profile, SQLite); the whole suite with `-race` on SQLite and PostgreSQL (22 packages); `golangci-lint` 0 issues. Survey after the slice: cognitive-complexity findings in `httpapi` 3 → 0 (`RegisterLiturgies` was 101), over-long functions in `httpapi` 3 → 0, repo-wide gocognit 19 → 16; `MapError` unchanged by plan ([R-5]).

**Incident:** the WIP commit of this slice was made on `main` by mistake (a background command does not keep its working directory, and `git add -A` ran in the main checkout). It held only `.claude/skills/stream-coding/SKILL.md` and `docs/impl/step3-adversarial-review.md`, was never pushed, and was undone by the owner with `git reset --mixed`. From then on `git -C <worktree>` and explicit paths are used for every commit from a background shell.

## 9. Slice 16B as built (2026-10-13)

| Part | Where |
|---|---|
| `Build` is a `builder` with named steps: `add`, `repeat`, `nextVerse`, `labelled` (rule 3), `number` (the verse numbering), `unlabelledBlock` (rules 4 and 5); the two scans before the main loop became one | `adapters/importers/paste/build.go` |
| `parseSong` is a `songParser`: `line`, `directive`, `startBlock`, `closeBlock`, `flushLoose`, `finish` | `adapters/importers/chordpro/chordpro.go` |
| `Parse` is `read` (a `reader` with `start`, `end`, `chars` that fills a `raw`) and `draftOf` (a `drafter` with `metadata`, `items`, `arrangement`) | `adapters/importers/openlyrics/openlyrics.go` |

The code inside each step is the original, moved; the extra field is only the state that the closures shared.

**Guard.** These are pure functions, so the old and new code were compared on generated inputs, 40 000 per importer: random block and label sequences for `paste.Build`; random directive files for ChordPro (tab and grid blocks, unclosed blocks, `{new_song}`, bad keys and CCLI numbers); generated XML for OpenLyrics (truncated, wrong root, DOCTYPE, bad entity, unsupported encoding, text split by XML comments, verse orders that do and do not match). Every candidate, warning and error was identical. The comparison was checked by making twelve deliberate behaviour changes in the new code: nine were noticed at once; one (a property or title split into several XML tokens) was not, and the generator was extended until it was; two are equivalent changes, because the original has two redundant guards (`#` lines inside a skipped block can never match the directive pattern, and `seenDirective` repeats what `warn` already does). Per R-1 those guards stay. The temporary copy of the old code and the comparison were deleted and not committed.

**Other checks:** the existing importer tests; `go test -race` for the importers, `app` (its import tests) and `server`; uncovered importer statements 29 before and 25 after (merged profile); `golangci-lint` 0 issues. Survey after the slice: no findings in the importer packages; repo-wide gocognit 19 at the start, 13 now; nestif 3, now 1.

**Not done:** no PostgreSQL run (the importers touch no database; the `app` import tests ran on SQLite); no permanent differential test, since it needs the old code; `paste.CharCount` has no callers or tests and was left alone (not in the plan).

## 10. Slice 16C.1 as built (2026-10-13): publishing, review, undo

| Part | Where |
|---|---|
| `buildPublished` is a `copyBuilder` with `item`, `song`, `assignments` and a `tooLarge` function; the repository calls are in the same order | `app/liturgy_publish.go` |
| `Review` calls `reviewAllowed` (the checks, in their old order), `reviewSubmitChecks` and `explainNoTransition` | `app/liturgy_review.go` |
| `step` is the transaction call and the bookkeeping of a refusal; `stepIn` is one undo or redo, with `pickEdit` and `refuseStep` | `app/liturgy_undo.go` |
| `songsOf` calls `sectionsOfSong` and `entryOf` | `app/liturgy_undo.go` |

**Tests first (their own commits):** `TestUndoRestoresSongs` (songs, sequence, keys and notes come back), `TestUndoSongReferencesGone` (refused with `reference_gone` when the song, a section or the singing part was deleted, nothing restored), `TestReviewUnknownAction`, `TestPublishSizeNamesLargestItem` (the largest item is named when it is not the first), `TestPublishMemberWithTwoDuties` (twice on the copy, once in the index). Each was checked to reach the line it was written for before any code moved.

**Mutation checks (17):** 11 caught. 6 survived, and each is a guard that is redundant or defensive in the original too: the archived check in `reviewAllowed` (the conditional update refuses an archived liturgy anyway, and `explainNoTransition` then says so); the "state changed meanwhile" branch of `explainNoTransition` (not reached by four concurrent publishers on PostgreSQL either, because the row lock serializes them; it would need fault injection in the store); the start position of a redo (a foreign edit that touches the item cannot lie between the edit and its undo, or the undo would have been refused); the editable check in `stepIn` (`openEdit` refuses first); the empty song ID guard of `sectionsOfSong` (a song in use cannot be deleted); the song cache (only an optimisation). One more survivor, the de-duplication of members on the copy, had no test; `TestPublishMemberWithTwoDuties` caught it afterwards.

**Other checks:** `go test -race` on `app`, SQLite and PostgreSQL (379 s); the server tests on SQLite; `golangci-lint` 0 issues; uncovered statements in the three files 136 before, 130 after (merged profile, SQLite). Survey: no findings left in these files; repo-wide gocognit 19 at the start, 9 now.

**Not done:** the defensive branches above are not pinned; `AddAssignment`, `PrepareCreateArchiving`, `Setup.Run` and `Auth.Login` belong to 16C.2 and 16C.3, with tests first.
