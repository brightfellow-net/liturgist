# 07 — Readings (Implementation)

> **Document type: Implementation.** Step 2 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slice 2B.
> Status: **Approved** 2026-10-03 (drafted 2026-10-02; Spec Gate and adversarial review round 3 done). Items marked **[P-xx]** are decisions listed in the [index](README.md#4-proposed-decisions).

## 1. Scope

The Bible reference parser, the readings store (reference + translation → text, attribution), the `BibleTextProvider` lookup order, and the readings pages. **The app ships with no Bible text** ([SPEC.md §5.4](../SPEC.md#54-readings)): every stored reading was typed or pasted by a member, or comes from a provider the church has registered.

Not in step 2: English and Mandarin book names (later, [SPEC.md §11](../SPEC.md#11-decisions-log)), different verse numbering between translations, Bible-file import and downloads of public-domain texts (after the MVP), and the "reading in use" check, which step 3 supplies.

## 2. References

### 2.1 Standard form [P-49]

A reference is stored as `<BOOK> <passage>` with the 66 **USFM book codes** (`GEN … REV`, table in §2.4). The passage has exactly one of these shapes:

| Shape | Examples (standard form) | Typed examples |
|---|---|---|
| One chapter | `PSA 23` | `Mzm 23`, `mazmur 23` |
| Chapter range | `PSA 23-25` | `Mzm 23–25` |
| Verses of one chapter, as ranges separated by `,` | `JHN 3:16`, `JHN 3:16-21`, `MAT 5:3,5-7` | `Yoh 3:16`, `Yoh 3:16–21`, `Mat. 5:3, 5-7` |
| Across chapters | `GEN 1:1-2:3` | `Kej. 1:1–2:3` |

Rules:

- Book names are matched **case-insensitively, ignoring dots and spaces** (`Kej.`, `kej`, `1 Kor`, `1Kor`). A leading Roman numeral `I`, `II`, `III` is the same as `1`, `2`, `3` (`II Korintus` = `2 Korintus`).
- Dashes `-`, `–`, `—` are the same. Chapter and verse are separated by `:` only; dots are ignored in the book name but a dot between numbers (`3.16`) is `bad_number`.
- Accepted book spellings: the Indonesian full name, the LAI abbreviation, extra aliases, and the USFM code itself (so the standard form parses back to itself). The table in §2.4 is the source for the first two; aliases are data in `domain/books.go`. A spelling that could mean two books is a bug (TC-R-002 checks every alias is unique). Matching is exact, never "prefix".
- **One-chapter books** (`OBA`, `PHM`, `2JN`, `3JN`, `JUD`): a passage without `:` is a list of **verses** of chapter 1: `Yud 3` is `JUD 1:3`, `Yud 3-5` is `JUD 1:3-5`. `Yud 1:3` is accepted too.
- Chapters are 1–150 and verses 1–176. Numbers must ascend: each range starts after the previous one ended; a range's end is not before its start. The parser checks **structure only**; it has no versification data, so `JHN 3:99` is accepted [P-49].
- Verse letters (`3:16a`), several chapters separated by `;` and whole books are not accepted in step 2 (reason `unsupported`).
- **Input limit:** the input is at most 100 characters, checked first, before any normalisation; longer input is `invalid_reference` with reason `too_long`. The HTTP endpoints apply the same limit to their `input` and `reference` parameters.
- `reference_display` is what the user typed, trimmed, with runs of spaces collapsed (at most 100 characters, by the limit above).

### 2.2 Parse result and errors

`domain.ParseReference(input) (Reference, error)`; `Reference` has `Book` (code), `Passage` (the canonical text after the code), `String()` (standard form), `Canonical(lang)` (book name in Indonesian, e.g. `Yohanes 3:16-21`; English names come later).

Errors carry a `reason`: `empty`, `too_long`, `unknown_book`, `missing_chapter`, `bad_number`, `bad_range`, `unsupported`.

### 2.3 Where the parser runs

Only on the server. The web app calls `GET /readings/parse` for its live preview, so there is a single parser to test and no drift between browser and server.

### 2.4 Books

Canonical Bible order (the `#` column is the sort ordinal used everywhere books are ordered), USFM code, Indonesian name (LAI, Terjemahan Baru), LAI abbreviation. Confirmed by the owner 2026-10-02; aliases found in the pilot church's documents are added to `domain/books.go` (and to TC-R-001) as they turn up.

| # | Code | Name | Abbr. |
|---|---|---|---|
| 1 | GEN | Kejadian | Kej |
| 2 | EXO | Keluaran | Kel |
| 3 | LEV | Imamat | Im |
| 4 | NUM | Bilangan | Bil |
| 5 | DEU | Ulangan | Ul |
| 6 | JOS | Yosua | Yos |
| 7 | JDG | Hakim-hakim | Hak |
| 8 | RUT | Rut | Rut |
| 9 | 1SA | 1 Samuel | 1Sam |
| 10 | 2SA | 2 Samuel | 2Sam |
| 11 | 1KI | 1 Raja-raja | 1Raj |
| 12 | 2KI | 2 Raja-raja | 2Raj |
| 13 | 1CH | 1 Tawarikh | 1Taw |
| 14 | 2CH | 2 Tawarikh | 2Taw |
| 15 | EZR | Ezra | Ezr |
| 16 | NEH | Nehemia | Neh |
| 17 | EST | Ester | Est |
| 18 | JOB | Ayub | Ayb |
| 19 | PSA | Mazmur | Mzm |
| 20 | PRO | Amsal | Ams |
| 21 | ECC | Pengkhotbah | Pkh |
| 22 | SNG | Kidung Agung | Kid |
| 23 | ISA | Yesaya | Yes |
| 24 | JER | Yeremia | Yer |
| 25 | LAM | Ratapan | Rat |
| 26 | EZK | Yehezkiel | Yeh |
| 27 | DAN | Daniel | Dan |
| 28 | HOS | Hosea | Hos |
| 29 | JOL | Yoel | Yl |
| 30 | AMO | Amos | Am |
| 31 | OBA | Obaja | Ob |
| 32 | JON | Yunus | Yun |
| 33 | MIC | Mikha | Mi |
| 34 | NAM | Nahum | Nah |
| 35 | HAB | Habakuk | Hab |
| 36 | ZEP | Zefanya | Zef |
| 37 | HAG | Hagai | Hag |
| 38 | ZEC | Zakharia | Za |
| 39 | MAL | Maleakhi | Mal |
| 40 | MAT | Matius | Mat |
| 41 | MRK | Markus | Mrk |
| 42 | LUK | Lukas | Luk |
| 43 | JHN | Yohanes | Yoh |
| 44 | ACT | Kisah Para Rasul | Kis |
| 45 | ROM | Roma | Rm |
| 46 | 1CO | 1 Korintus | 1Kor |
| 47 | 2CO | 2 Korintus | 2Kor |
| 48 | GAL | Galatia | Gal |
| 49 | EPH | Efesus | Ef |
| 50 | PHP | Filipi | Flp |
| 51 | COL | Kolose | Kol |
| 52 | 1TH | 1 Tesalonika | 1Tes |
| 53 | 2TH | 2 Tesalonika | 2Tes |
| 54 | 1TI | 1 Timotius | 1Tim |
| 55 | 2TI | 2 Timotius | 2Tim |
| 56 | TIT | Titus | Tit |
| 57 | PHM | Filemon | Flm |
| 58 | HEB | Ibrani | Ibr |
| 59 | JAS | Yakobus | Yak |
| 60 | 1PE | 1 Petrus | 1Ptr |
| 61 | 2PE | 2 Petrus | 2Ptr |
| 62 | 1JN | 1 Yohanes | 1Yoh |
| 63 | 2JN | 2 Yohanes | 2Yoh |
| 64 | 3JN | 3 Yohanes | 3Yoh |
| 65 | JUD | Yudas | Yud |
| 66 | REV | Wahyu | Why |

## 3. The readings store

Domain type `domain.Reading`; table in [reference/schema.md](../reference/schema.md#readings).

| Field | Rule |
|---|---|
| `reference` | Standard form ([§2.1](#21-standard-form-p-49)) |
| `reference_display` | As typed ([§2.1](#21-standard-form-p-49)) |
| `translation` | A `translations.code` ([schema](../reference/schema.md#translations)); unknown → 422. The reading's language is the translation's |
| `text` | 1–20000 characters after the normalisation of [06 §2.2](06-song-library.md#22-section) |
| `attribution` | 0–300 characters; the credit line shown on the published view and in the PDF |
| `source_provider` | Set by the server and never taken from a request: `manual` for `POST /readings`, the provider's ID for `POST /readings/from-provider` [P-50] |
| `version` | Integer, +1 on every change; changes are conditional on it, as in [06 §2.4](06-song-library.md#24-editing-sections-and-concurrency-p-46) |

- **Unique per church + `reference` + translation** (`readings_church_ref_key`). A second `POST` → 409 `reading_exists` with the existing `reading_id`. The same passage in another translation is a separate reading.
- Changing the reference means a new reading: `PATCH` cannot change `reference` or `translation`.
- Text is the church's own entry; the app never checks it against a Bible. It is not logged.

### 3.1 Lookup order and storing provider text [P-50]

`GET /readings/lookup` finds text for a reference in this order, stopping at the first hit:

1. A stored reading of this church for the standard reference and translation.
2. Each registered `BibleTextProvider`, in registration order: `Lookup(ctx, ref, translation)` returns `BibleText{Text, Attribution, Source, MayStore}`; `ErrNotAvailable` means "try the next".
3. Nothing: the response has `reading: null, provider: null`, and the web form shows the paste box.

The community build registers no provider, so only step 1 happens.

**Provider failures.** Every `Lookup` gets a **5-second deadline** derived from the request's context (`Readings.ProviderTimeout`, zero meaning 5 s). The call runs in its own goroutine and the use case stops waiting at the deadline, so a provider that ignores its context cannot hold the request; its goroutine ends when the provider returns. Any error other than `ErrNotAvailable`, a timeout, and a result that breaks a limit (`Source` different from the registered provider ID, text over 20000 characters, attribution over 300) are logged at warn level (provider ID and error class, never text) and treated as "not available" for this lookup; the next provider is still tried. When no stored reading was found and at least one provider failed this way, the response has `provider_error: true`, and the web app says the Bible text source could not be reached and that the text can still be pasted.

**Storing provider text is a server decision.** Provider text is saved only by `POST /readings/from-provider { reference, translation, provider }`: the server repeats the lookup with that provider and saves the text and attribution **it receives from the provider**, and only when `may_store` is true (otherwise 422 `validation_failed`, field `provider`). `POST /readings` has no `source_provider` field (a request containing one is rejected as an unknown field, 422) and always records `manual`. So a client can neither label text as coming from a provider nor make the app persist provider text that forbids storing.

**What this cannot prevent:** a member can still type or paste such text by hand into `POST /readings`. Manual entry is the church's own act and needs no licence from the app ([SPEC.md §5.4](../SPEC.md#54-readings)); the rule only guarantees that *the app* never stores text a provider forbids storing.

`suggested_attribution` in the lookup response is the attribution of this church's most recently updated reading in the same translation, so the credit line is typed once per translation.

## 4. API

All paths under `/api/v1`. **View** = any member; **edit** = `library.edit`.

| Method & path | Scope | Request | Response |
|---|---|---|---|
| `GET /readings/parse` | view | query `input` | 200 `{ reference, canonical, display }` or 422 `invalid_reference` with `reason` |
| `GET /readings/lookup` | view | query `reference` (typed), `translation` (default: the church's) | `{ reference, canonical, display, translation, reading: Reading \| null, provider: { text, attribution, source, may_store } \| null, provider_error, suggested_attribution }`; unparsable reference → 422 `invalid_reference` |
| `GET /readings` | view | query `q`, `translation`, `limit` (default 50, max 100), `offset` | `{ items: [ReadingSummary], total }`; summary = `id`, `reference`, `canonical`, `reference_display`, `translation`, a 120-character `snippet`, `actions` |
| `GET /readings/{id}` | view | — | Reading with `text`, `attribution`, `source_provider`, `version`, `actions: { edit, delete }` |
| `POST /readings` | edit | `{ reference, translation?, text, attribution? }` (no `translation`: the church's default) | 201 Reading with `source_provider: "manual"`; 409 `reading_exists` |
| `POST /readings/from-provider` | edit | `{ reference, translation, provider }` | 201 Reading with `source_provider` = the provider's ID; 409 `reading_exists`; 422 `validation_failed` if the provider has no text or `may_store` is false |
| `PATCH /readings/{id}` | edit | `{ version, text?, attribution?, reference_display? }`; a `reference_display` must still parse to the reading's own reference, otherwise 422 `validation_failed` (field `reference_display`) | Reading; 409 `version_conflict` |
| `DELETE /readings/{id}` | edit | — | 204; 409 `reading_in_use` if `ReadingUsage.ReadingInUse` |

**Authorization and `actions`:** every route needs a session and membership of the church; `POST`, `PATCH`, `DELETE` and `from-provider` need `library.edit` (403 `forbidden`). `actions` is `{ edit, delete }` on `Reading` and `ReadingSummary`, both true exactly when the member holds `library.edit`.

`q` is folded ([06 §5.1](06-song-library.md#51-folding)) and matched as a **substring** of the folded reference, canonical name and text, with `instr` (SQLite) or `strpos` (PostgreSQL) so that `%`, `_` and `\` in `q` are literal; readings are few, so no full-text index. A `q` that folds to nothing (only punctuation) lists every reading, as in [06 §5.2](06-song-library.md#52-query-semantics-p-48). Order: book ordinal (the `#` column of §2.4), then the first chapter and verse of the passage, then translation code, then the standard reference (so `JHN 3:16` comes before `JHN 3:16-21`), then ID. The use case sorts and pages the matching rows in memory: the `readings` table has no ordinal columns, and readings are few.

Logging: info lines `reading_created`, `reading_updated`, `reading_deleted` with IDs; never text.

## 5. Pages

| Route | Page | Who |
|---|---|---|
| `/library/readings` | Second tab of the library (the songs list is the first; both sit under one "Library" heading with the two tabs): list with search and a translation filter; empty state explains "Type a reference and paste the text once; next time it's filled in" with the action "Add a reading" | View |
| `/library/readings/new` | Reference input with a live preview of how it was understood ("Yohanes 3:16-21"), translation select (the church's default pre-selected), text box, attribution (pre-filled from `suggested_attribution`). If the reading already exists the page says so and links to it; if a provider has the text it is offered with its attribution and saved with "Save this text" (`POST /readings/from-provider`; the button is absent when `may_store` is false) | `library.edit` |
| `/library/readings/{id}` | The reading, attribution below it; "Edit" (text and attribution, in place on the same page; the reference and translation cannot change) and "Delete" per `actions` | View |

The preview calls `GET /readings/parse` after the user stops typing for 400 ms and on leaving the field; an error shows the translated reason (`unknown_book`: "I don't know this book. Try the usual short name, e.g. Yoh or Mzm."). Book names in the preview are Indonesian in both UI languages in step 2 [P-49].

## 6. Ports

| Port | Signature | Implementation | Used by |
|---|---|---|---|
| `ChurchStore.Readings()` | create, update (conditional on `version`), delete, `ByID`, `ByReference`, `List`, `LatestAttribution(translation)` | `adapters/sqlstore` | Reading use cases |
| `BibleTextProvider` | `ID() string` and `Lookup(ctx, ref domain.Reference, translation string) (BibleText, error)` (`Lookup` exists since step 1, `ID` is added in step 2 so that a stored reading and `POST /readings/from-provider` can name the provider, [04 §7](04-tenancy-extensions.md#7-extension-points)) | None registered; `server.WithBibleTextProvider(p)` appends one; 5 s deadline per call (§3.1) | Lookup |
| `ReadingUsage` | `ReadingInUse(ctx, church, reading) (bool, error)` | Step 2: `app.NeverUsed`; step 3: queries the liturgies | Delete |

`domain.Reference` and `app.BibleText` stop being placeholders in this step (listed under **Ports** in `CHANGELOG.md`).

## 7. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Ship, bundle or fetch Bible text | Only text a member typed or a registered provider returned | Translations are copyrighted ([SPEC.md §5.4](../SPEC.md#54-readings)) |
| Parse references in the browser too | One parser in Go; the web app calls `/readings/parse` | Two parsers drift apart |
| Match book names by prefix or fuzzily | Exact aliases from `domain/books.go`; an unknown spelling is an error | "Yo" could be four books; wrong guesses put the wrong reading in a liturgy |
| Store the typed reference as the key | Store the standard form; keep the typed text only for display | `Yoh 3:16` and `Yohanes 3.16` must find the same reading |
| Overwrite a stored reading when the same reference is added again | 409 `reading_exists` with a link to the existing one | Two members must not lose each other's text or attribution |
| Take the source of a stored reading from the request, or save provider text the client sends | `POST /readings/from-provider` re-fetches the text from the provider and checks `may_store` on the server | A client-supplied label cannot be trusted; only the server knows what the provider returned |
| Claim to validate verse numbers | Check structure only and say so in the docs and tests | No versification data exists in step 2 |
| Log reading text | IDs only | Size and copyright |

## 8. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-R-001 | `ParseReference` | `Yoh 3:16-21` → `JHN 3:16-21`; `Kej. 1:1–2:3` → `GEN 1:1-2:3`; `Mzm 23` → `PSA 23`; `Mzm 23–25` → `PSA 23-25`; `Mat. 5:3, 5-7` → `MAT 5:3,5-7`; `1 Kor 13` and `I Kor 13` and `1Kor13`… → `1CO 13`; `II Korintus 5:17` → `2CO 5:17`; `Yud 3` → `JUD 1:3`; `JHN 3:16` → `JHN 3:16` | As listed | Upper/lower case, extra spaces, tabs, non-breaking space, `—` as dash |
| TC-R-002 | Book table | All 66 books | Every book has a code, a name and an abbreviation; every alias and name is unique across books; every name and abbreviation parses to its own code | `Hak` vs `Hag`; `Yl`; `Am` |
| TC-R-003 | Errors | `""`, `Foo 1`, `Yoh`, `Yoh 3:0`, `Yoh 151`, `Yoh 3:21-16`, `Yoh 3:16,16`, `Yoh 3:16a`, `Yoh 3;4`, `Yoh 3.16` | `empty`, `unknown_book`, `missing_chapter`, `bad_number`, `bad_number`, `bad_range`, `bad_range`, `unsupported`, `unsupported`, `bad_number` | 101 characters → `too_long` (exactly 100 valid characters are accepted); the `too_long` check runs before alias matching |
| TC-R-004 | Round trip | Every case of TC-R-001 | `ParseReference(r.String()).String() == r.String()` | — |
| TC-R-005 | Reading validation | Each rule in §3 | As specified | 20000 and 20001 characters; attribution 300/301; unknown translation code |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-R-001 | Create, read, edit, delete | Editor and team member | Team member can view only; the same passage typed two ways is one reading; `PATCH` with an old `version` → 409 `version_conflict` | — |
| IT-R-002 | Uniqueness | One reading | Same standard reference and translation → 409 `reading_exists` (with `reading_id`); another translation succeeds; another church's reading is invisible (404) | — |
| IT-R-003 | Lookup order and storing | Fake provider returning `may_store = true`, then `false`; a stored reading | Stored reading first, then provider; `from-provider` saves the provider's text with `source_provider` = its ID; with `may_store = false` → 422 and nothing saved; `POST /readings` with a `source_provider` field → 422; text posted to `POST /readings` is recorded as `manual`; `ErrNotAvailable` falls through to `reading: null` | — |
| IT-R-004 | Suggested attribution | Two readings in TB | The newer reading's attribution is suggested for TB, none for KJV | — |
| IT-R-005 | List and search | Readings in `id` and `zh-Hans` | Order by book, chapter, verse; substring search in reference and text; translation filter | — |
| IT-R-006 | Delete hook | `ReadingUsage` stub answering "in use" | 409 `reading_in_use`, nothing deleted | — |
| IT-R-007 | Constraints | Direct inserts bypassing the app | Duplicate (church, reference, translation), a reading in another church's name, an unknown translation are rejected on both dialects | — |
| IT-R-008 | Provider failures | Providers that time out, return an error, return a wrong `Source`, return 20001 characters | Each is skipped after at most 5 s and logged without text; a later provider is still tried; `provider_error: true` only when nothing was found | — |

Web tests: unit tests for the reference preview (debounce, reason messages) and the "already exists" state; end-to-end test E2E-W-009 (add a reading by typing "Yoh 3:16-21", see it understood, save, find it by searching, add the same reference with different spelling and be told it exists); axe covers the new pages.

## 9. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| `invalid_reference` | 422, `reason` | `unknown_book`: "I don't know this book. Try the usual short name, e.g. Yoh or Mzm." `missing_chapter`: "Add a chapter, e.g. Yoh 3." `bad_number`: "Check the chapter and verse numbers." `bad_range`: "The verses must go in order, e.g. 16-21." `unsupported`: "This kind of reference isn't supported yet. Use one passage, e.g. Yoh 3:16-21." `empty`: "Type a reference." `too_long`: "That reference is too long." | Fix the input |
| `provider_error` flag | `GET /readings/lookup` | "The Bible text source could not be reached. You can still paste the text." | Paste the text |
| `reading_exists` | 409 | "You already saved this reading." | Link to the existing reading |
| `version_conflict` | 409 | "This reading was changed by someone else. Reload to see their version." | Reload; input kept |
| `reading_in_use` | 409 | "This reading is used in a liturgy that isn't published yet, so it can't be deleted." | Stay on page |
| `validation_failed` | 422 | "Check this field." | Fix and resubmit |
| `forbidden` / `not_found` | 403 / 404 | As elsewhere | — |

## 10. References

| Topic | Location |
|---|---|
| Readings and Bible text decisions | [SPEC.md §5.4](../SPEC.md#54-readings), [§11](../SPEC.md#11-decisions-log) |
| `BibleTextProvider` | [04 §7](04-tenancy-extensions.md#7-extension-points) |
| Tables | [reference/schema.md](../reference/schema.md#readings) |
| Folding, concurrency | [06 §5.1](06-song-library.md#51-folding), [06 §2.4](06-song-library.md#24-editing-sections-and-concurrency-p-46) |
