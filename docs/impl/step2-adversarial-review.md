# Adversarial Review — Liturgist Step 2 Specification

Reviewed document: [`review-bundle.md`](../../review-bundle.md)  
Review date: 2026-10-03

This review covers the proposed Step 2 song library, readings, importing, and schema material included in the bundle.

## Critical findings

### [CRITICAL] Failed imports contradict the batch-closing rule

**Location:** 08 §2, §3, §5; IT-I-002

**Problem:** A batch closes “when nothing is `pending`.” A candidate that fails Apply still has an `accept` or `merge` decision, so it is no longer pending. But IT-I-002 requires the batch to stay open after a candidate fails. Those rules produce different retry behavior.

**Fix:** Define an explicit candidate state for failures and specify whether failed candidates are retryable. Close a batch only when every candidate is terminal, with a clear definition of terminal.

### [CRITICAL] A provider’s “may not store” restriction is client-bypassable

**Location:** 07 §3.1 and §4; `POST /readings`

**Problem:** The UI hides Save when a provider returns `may_store = false`, and the API rejects a provider ID marked non-storable. But the request otherwise accepts arbitrary text; a client can submit that same text as `manual` or omit `source_provider`. The API has no specified way to verify where the text came from.

**Fix:** Define a server-enforced storage authorization model. If provider text must not be stored, do not expose it through a path that allows persistence, and do not rely on a client-supplied source label to enforce the rule.

## High findings

### [HIGH] Search parity is asserted without a shared tokenization contract

**Location:** 06 §5.1–5.3

**Problem:** The spec promises identical matches from SQLite FTS5 `unicode61` and PostgreSQL `to_tsvector('simple')`, but does not define equivalent tokenization or query construction for either engine. A shared fold function does not guarantee that the database tokenizers and prefix queries return the same matches, especially for Unicode text.

**Fix:** Define the exact token and query semantics independently of the database, then specify implementations that satisfy them. Add cross-dialect tests for Unicode, one-character prefixes, mixed scripts, and punctuation boundaries.

### [HIGH] Hymnal “exact match” has incompatible key formats

**Location:** 06 §2.1 and §5.2–5.3

**Problem:** Stored `hymnal_key` is `<SOURCE KEY>:<number>`, while the example query `kj 12` folds to `kj 12`. The spec does not say how the query is converted to the stored key before exact comparison.

**Fix:** Define one canonical parser and key format for both stored values and queries, with examples showing that `kj 12` and `pkj12a` resolve to their intended keys.

### [HIGH] Merge can silently delete edited sections

**Location:** 08 §3 “Merge”; §6

**Problem:** Merge replaces the target song’s sections with the imported draft and deletes unmatched existing sections. The UI describes this as “Merge into” and shows a duplicate suggestion, but the spec does not require a before/after diff or confirmation that existing sections will be removed. This can destroy church-edited lyrics.

**Fix:** Require a review of the section replacement and explicit confirmation of deletions, or define a non-destructive merge that preserves unmatched existing sections.

### [HIGH] Merge can overwrite concurrent edits made after review

**Location:** 08 §2–3

**Problem:** The candidate stores `merge_into`, but no target song version is captured when the user reviews or chooses the merge. If the target changes before Apply, Apply can replace its sections using stale reviewed input.

**Fix:** Capture the target song’s version when the merge decision is saved and reject Apply with a conflict if that version changed. Require the user to review the latest target before retrying.

### [HIGH] Song group changes bypass the song-version contract

**Location:** 06 §2.1, §2.3–2.4, and §3

**Problem:** A song’s version is supposed to increment on every change, but link/unlink operations have no version in their request and do not say whether they increment either affected song’s version. This makes the meaning of `version` inconsistent and leaves stale clients unaware of group changes.

**Fix:** Specify version checks and increments for link/unlink, language changes, and all other mutations. Define how a link touching two songs detects and reports conflicts.

### [HIGH] Song arrangements have no referential integrity

**Location:** 06 §2.1–2.4; schema section in the bundle

**Problem:** `default_arrangement` is JSON containing section IDs, but there is no FK or database check that those IDs belong to the same song or still exist. The prose requires application cleanup when a section is deleted, so one missed code path can leave broken arrangements—and future liturgy references are meant to depend on stable section IDs.

**Fix:** Normalize arrangements into a join table with composite same-song FKs, or explicitly define and test a single transactional invariant checker for every section mutation and repair path.

### [HIGH] File bodies can exceed the intended per-file processing limits

**Location:** 08 §4–5

**Problem:** The request has a 6 MiB total body limit and a 200-file limit, but the 200,000-character cap applies only to pasted lyrics. No per-file text limit or aggregate parsed-output limit is specified for OpenLyrics or ChordPro. A single file can consume most of the request budget and generate large parser work or candidate data.

**Fix:** Set per-file byte and character limits, aggregate decoded-text limits, and parser CPU/memory limits for every format. Define whether rejected files fail individually or reject the whole request when a resource limit is exceeded.

### [HIGH] Section-matching heuristics can preserve the wrong IDs

**Location:** 08 §3 “Merge”

**Problem:** Non-verse sections match by kind, taking the “first unused match in order.” A draft with multiple choruses, tags, or bridges can inherit IDs from semantically different existing sections. Since IDs are stable and later liturgies will reference them, the wrong match can silently change what a future reference points to.

**Fix:** Define a deterministic semantic matching rule and require a human-visible ID mapping when multiple plausible matches exist. Do not silently reuse an ID where identity is uncertain.

### [HIGH] Import parsing has no clear partial-file failure boundary

**Location:** 08 §4.2–4.4

**Problem:** ChordPro can contain several songs per file, while OpenLyrics is one song per file. The error model rejects unreadable files as a whole, but does not say what happens if one song inside a multi-song file is malformed or exceeds limits. Implementations could discard the entire file or keep only some songs.

**Fix:** Define failure granularity for multi-song files and specify whether valid songs from a partially invalid file are retained, rejected, or surfaced as separate candidates with errors.

### [HIGH] SQL text search ordering is underspecified despite a precise ranking promise

**Location:** 06 §5.2–5.3

**Problem:** The spec separates matches into title/alternate-title/hymnal and lyrics groups, then says PostgreSQL uses a weighted `tsvector`; it does not define the weights, how alternate titles are distinguished, or the exact query construction. The schema’s PostgreSQL vector combines the fields, so an implementer cannot reproduce the stated groups from the described data unambiguously.

**Fix:** Specify separate searchable vectors or field weights and exact rank/group predicates for both engines. Add tests asserting identical ordered result IDs, not merely that selected songs are found.

### [HIGH] Paste splitting can misclassify repeated material without a defined mapping

**Location:** 08 §4.1 rules 2–5

**Problem:** When an unlabelled block repeats a previously labelled section, rule 4 says to drop the copy, while rule 5 says the order of appearance becomes the arrangement whenever blocks were dropped. The spec does not explicitly say whether that dropped block adds a reference to the earlier section in the arrangement. The same ambiguity exists when repeated unlabelled blocks are collapsed into a chorus.

**Fix:** Define, for each deduplication case, whether the repeated occurrence adds an arrangement entry and which section ID/key it maps to. Add exact input/output examples for labelled and unlabelled repeats.

### [HIGH] Unknown ChordPro directives are silently discarded despite the warning rule

**Location:** 08 §4.3 and §8

**Problem:** The anti-pattern says every guess must produce a warning, but ChordPro says other directives are ignored without distinguishing harmless metadata from directives containing lyrics or structure the parser does not understand. Users can lose content without seeing a warning.

**Fix:** Enumerate ignored directives and define which produce warnings. Warn on any unsupported directive that may carry content or affect section structure.

## Medium findings

### [MEDIUM] Folded text may not have the specified diacritic behavior

**Location:** 06 §5.1

**Problem:** The sequence says NFKC, lowercase, then remove combining marks “(NFD, drop Mn).” It is unclear whether NFD is applied after lowercasing or merely described parenthetically. Without decomposition, precomposed accented characters retain their marks and do not fold as expected.

**Fix:** Specify the exact Unicode transform sequence, including the decomposition step and whether recomposition occurs. Add tests with precomposed and decomposed accented input.

### [MEDIUM] Song creation and update field semantics are inconsistent

**Location:** 06 §2.1 and §3

**Problem:** `default_arrangement` says empty means “all sections in order,” but also says the empty arrangement is a valid 0-entry list. The API does not define how a caller explicitly requests “all sections” versus no arrangement, or what a PATCH clearing the arrangement means.

**Fix:** Choose one representation for implicit “all sections” and define create/PATCH behavior for omitted, empty, and explicit arrangement values.

### [MEDIUM] Reference sort order is not reliably specified by the book table

**Location:** 07 §2.4 and §4

**Problem:** Readings are ordered by “USFM code order in §2.4,” but the books are presented in three side-by-side columns. It is unclear whether order means row order, column order, or canonical Bible order.

**Fix:** Provide an explicit ordinal for each book and use it as the canonical sort key.

### [MEDIUM] Reference parser limits leave expensive or oversized input behavior unclear

**Location:** 07 §2.1–2.3; TC-R-003

**Problem:** `reference_display` is capped at 100 characters, but the parser’s input limit is only tested as “100+ character input.” The spec does not say whether parsing rejects long input before alias matching or how the API bounds the query parameter.

**Fix:** State an exact parser input limit and return reason, enforce it before normalization, and apply the same limit to the HTTP endpoint.

### [MEDIUM] Search reindex has no concurrent-write or cutover semantics

**Location:** 06 §5.3

**Problem:** `liturgist search reindex` rebuilds the complete index, but the spec does not say whether it holds `LockChurch`, blocks writes, or replaces the index atomically. Concurrent edits could be lost from the rebuilt index or leave `song_search` and FTS rows inconsistent.

**Fix:** Define an atomic rebuild/cutover strategy for each database and its interaction with concurrent song writes. Test reindex racing with create, update, and delete.

### [MEDIUM] Import Apply can race with candidate edits and decisions

**Location:** 08 §2 and §5

**Problem:** Apply re-reads candidate state and serializes per church, but candidate draft/decision PATCH operations are not specified to use the same lock or a candidate version. Apply can therefore process a candidate while a reviewer is changing its draft or decision.

**Fix:** Add candidate versions and conditional updates, or require Apply and all candidate mutations to serialize on the batch and reject edits once Apply begins.

### [MEDIUM] Import status does not model in-progress or partially failed work

**Location:** 08 §2–3 and schema section

**Problem:** Batch status only has `open`/`closed`, while Apply operates one transaction per candidate and can return per-candidate failures. There is no state for an Apply in progress, and closure behavior after partial failure conflicts with IT-I-002.

**Fix:** Define the batch/candidate state machine, including retries, concurrent Apply, failures, and cleanup eligibility.

### [MEDIUM] Provider calls have no timeout or failure policy

**Location:** 07 §3.1 and §6

**Problem:** Lookup calls providers in registration order but does not define deadlines, cancellation, or treatment of errors other than `ErrNotAvailable`. One slow or failing provider can make lookup unavailable or prevent later providers from being tried.

**Fix:** Specify per-provider timeouts and error handling, whether non-availability errors stop or continue lookup, and how provider attribution/source are validated.

### [MEDIUM] Library and import authorization rules leave action checks underspecified

**Location:** 06 §3; 07 §4; 08 §5

**Problem:** APIs say every response carries `actions`, but the exact action fields and whether `library.edit` is required for every import operation—including read, delete, and candidate review—are not fully defined. The UI and API may diverge on what non-editors can see.

**Fix:** List action fields per resource and define route-by-route authorization, including whether a member can view an existing batch after losing `library.edit`.

### [MEDIUM] The step-2 migration number is not tied to the preceding migration version

**Location:** Schema section heading; migration conventions in 02-persistence.md

**Problem:** The schema says the migration is `00002_library.sql`, but the bundle does not establish which version step 1 consumes or whether `00002` is the next available version in both migration histories.

**Fix:** Name the exact preceding migration version and require both dialect histories to reserve/use the same next version.
