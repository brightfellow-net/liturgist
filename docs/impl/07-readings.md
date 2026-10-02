# 07 — Readings (Implementation)

> **Document type: Implementation.** Step 2 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slice 2B.
> Status: **Proposed** (draft 2026-10-02). Items marked **[P-xx]** are decisions listed in the [index](README.md#4-proposed-decisions).

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
- `reference_display` is what the user typed, trimmed, with runs of spaces collapsed, at most 100 characters.

### 2.2 Parse result and errors

`domain.ParseReference(input) (Reference, error)`; `Reference` has `Book` (code), `Passage` (the canonical text after the code), `String()` (standard form), `Canonical(lang)` (book name in Indonesian, e.g. `Yohanes 3:16-21`; English names come later).

Errors carry a `reason`: `empty`, `unknown_book`, `missing_chapter`, `bad_number`, `bad_range`, `unsupported`.

### 2.3 Where the parser runs

Only on the server. The web app calls `GET /readings/parse` for its live preview, so there is a single parser to test and no drift between browser and server.

### 2.4 Books

Indonesian name (LAI, Terjemahan Baru), LAI abbreviation, USFM code. **The abbreviations are to be confirmed against the pilot church's documents** before approval.

| Code | Name | Abbr. | Code | Name | Abbr. | Code | Name | Abbr. |
|---|---|---|---|---|---|---|---|---|
| GEN | Kejadian | Kej | PRO | Amsal | Ams | ROM | Roma | Rm |
| EXO | Keluaran | Kel | ECC | Pengkhotbah | Pkh | 1CO | 1 Korintus | 1Kor |
| LEV | Imamat | Im | SNG | Kidung Agung | Kid | 2CO | 2 Korintus | 2Kor |
| NUM | Bilangan | Bil | ISA | Yesaya | Yes | GAL | Galatia | Gal |
| DEU | Ulangan | Ul | JER | Yeremia | Yer | EPH | Efesus | Ef |
| JOS | Yosua | Yos | LAM | Ratapan | Rat | PHP | Filipi | Flp |
| JDG | Hakim-hakim | Hak | EZK | Yehezkiel | Yeh | COL | Kolose | Kol |
| RUT | Rut | Rut | DAN | Daniel | Dan | 1TH | 1 Tesalonika | 1Tes |
| 1SA | 1 Samuel | 1Sam | HOS | Hosea | Hos | 2TH | 2 Tesalonika | 2Tes |
| 2SA | 2 Samuel | 2Sam | JOL | Yoel | Yl | 1TI | 1 Timotius | 1Tim |
| 1KI | 1 Raja-raja | 1Raj | AMO | Amos | Am | 2TI | 2 Timotius | 2Tim |
| 2KI | 2 Raja-raja | 2Raj | OBA | Obaja | Ob | TIT | Titus | Tit |
| 1CH | 1 Tawarikh | 1Taw | JON | Yunus | Yun | PHM | Filemon | Flm |
| 2CH | 2 Tawarikh | 2Taw | MIC | Mikha | Mi | HEB | Ibrani | Ibr |
| EZR | Ezra | Ezr | NAM | Nahum | Nah | JAS | Yakobus | Yak |
| NEH | Nehemia | Neh | HAB | Habakuk | Hab | 1PE | 1 Petrus | 1Ptr |
| EST | Ester | Est | ZEP | Zefanya | Zef | 2PE | 2 Petrus | 2Ptr |
| JOB | Ayub | Ayb | HAG | Hagai | Hag | 1JN | 1 Yohanes | 1Yoh |
| PSA | Mazmur | Mzm | ZEC | Zakharia | Za | 2JN | 2 Yohanes | 2Yoh |
| MAT | Matius | Mat | MAL | Maleakhi | Mal | 3JN | 3 Yohanes | 3Yoh |
| MRK | Markus | Mrk | LUK | Lukas | Luk | JUD | Yudas | Yud |
| JHN | Yohanes | Yoh | ACT | Kisah Para Rasul | Kis | REV | Wahyu | Why |

## 3. The readings store

Domain type `domain.Reading`; table in [reference/schema.md](../reference/schema.md#readings).

| Field | Rule |
|---|---|
| `reference` | Standard form ([§2.1](#21-standard-form-p-49)) |
| `reference_display` | As typed ([§2.1](#21-standard-form-p-49)) |
| `translation` | A `translations.code` ([schema](../reference/schema.md#translations)); unknown → 422. The reading's language is the translation's |
| `text` | 1–20000 characters after the normalisation of [06 §2.2](06-song-library.md#22-section) |
| `attribution` | 0–300 characters; the credit line shown on the published view and in the PDF |
| `source_provider` | `manual` (typed or pasted) or the provider's ID [P-50] |
| `version` | Integer, +1 on every change; changes are conditional on it, as in [06 §2.4](06-song-library.md#24-editing-sections-and-concurrency-p-46) |

- **Unique per church + `reference` + translation** (`readings_church_ref_key`). A second `POST` → 409 `reading_exists` with the existing `reading_id`. The same passage in another translation is a separate reading.
- Changing the reference means a new reading: `PATCH` cannot change `reference` or `translation`.
- Text is the church's own entry; the app never checks it against a Bible. It is not logged.

### 3.1 Lookup order [P-50]

`GET /readings/lookup` finds text for a reference in this order, stopping at the first hit:

1. A stored reading of this church for the standard reference and translation.
2. Each registered `BibleTextProvider`, in registration order: `Lookup(ctx, ref, translation)` returns `BibleText{Text, Attribution, Source, MayStore}`; `ErrNotAvailable` means "try the next".
3. Nothing: the response has `reading: null, provider: null`, and the web form shows the paste box.

The community build registers no provider, so only step 1 happens. A provider result with `MayStore = false` is shown but the save button is hidden and `POST /readings` with that provider as `source_provider` is refused (422). A provider text that may be stored is saved with `source_provider` set to the provider's ID, and the provider's attribution.

`suggested_attribution` in the lookup response is the attribution of this church's most recently updated reading in the same translation, so the credit line is typed once per translation.

## 4. API

All paths under `/api/v1`. **View** = any member; **edit** = `library.edit`.

| Method & path | Scope | Request | Response |
|---|---|---|---|
| `GET /readings/parse` | view | query `input` | 200 `{ reference, canonical, display }` or 422 `invalid_reference` with `reason` |
| `GET /readings/lookup` | view | query `reference` (typed), `translation` (default: the church's) | `{ reference, canonical, display, translation, reading: Reading \| null, provider: { text, attribution, source, may_store } \| null, suggested_attribution }`; unparsable reference → 422 `invalid_reference` |
| `GET /readings` | view | query `q`, `translation`, `limit` (default 50, max 100), `offset` | `{ items: [ReadingSummary], total }`; summary = `id`, `reference`, `canonical`, `reference_display`, `translation`, a 120-character `snippet`, `actions` |
| `GET /readings/{id}` | view | — | Reading with `text`, `attribution`, `source_provider`, `version`, `actions: { edit, delete }` |
| `POST /readings` | edit | `{ reference, translation, text, attribution?, source_provider? }` | 201 Reading; 409 `reading_exists` |
| `PATCH /readings/{id}` | edit | `{ version, text?, attribution?, reference_display? }` | Reading; 409 `version_conflict` |
| `DELETE /readings/{id}` | edit | — | 204; 409 `reading_in_use` if `ReadingUsage.ReadingInUse` |

`q` is folded ([06 §5.1](06-song-library.md#51-folding)) and matched as a **substring** of the folded reference, canonical name and text; readings are few, so no full-text index. Order: `canonical` book order (by USFM code order in §2.4), then chapter and verse, then translation.

Logging: info lines `reading_created`, `reading_updated`, `reading_deleted` with IDs; never text.

## 5. Pages

| Route | Page | Who |
|---|---|---|
| `/library/readings` | Second tab of the library: list with search and a translation filter; empty state explains "Type a reference and paste the text once; next time it's filled in" with the action "Add a reading" | View |
| `/library/readings/new` | Reference input with a live preview of how it was understood ("Yohanes 3:16-21"), translation select (the church's default pre-selected), text box, attribution (pre-filled from `suggested_attribution`). If the reading already exists the page says so and links to it; if a provider has the text it is offered with its attribution | `library.edit` |
| `/library/readings/{id}` | The reading, attribution below it; "Edit" and "Delete" per `actions` | View |

The preview calls `GET /readings/parse` after the user stops typing for 400 ms and on leaving the field; an error shows the translated reason (`unknown_book`: "I don't know this book. Try the usual short name, e.g. Yoh or Mzm."). Book names in the preview are Indonesian in both UI languages in step 2 [P-49].

## 6. Ports

| Port | Signature | Implementation | Used by |
|---|---|---|---|
| `ChurchStore.Readings()` | create, update (conditional on `version`), delete, `ByID`, `ByReference`, `List`, `LatestAttribution(translation)` | `adapters/sqlstore` | Reading use cases |
| `BibleTextProvider` | `Lookup(ctx, ref domain.Reference, translation string) (BibleText, error)` (exists since step 1, [04 §7](04-tenancy-extensions.md#7-extension-points)) | None registered | Lookup |
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
| Save provider text marked `may_store = false` | Show it, hide "Save", refuse the API call | Provider licences |
| Claim to validate verse numbers | Check structure only and say so in the docs and tests | No versification data exists in step 2 |
| Log reading text | IDs only | Size and copyright |

## 8. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-R-001 | `ParseReference` | `Yoh 3:16-21` → `JHN 3:16-21`; `Kej. 1:1–2:3` → `GEN 1:1-2:3`; `Mzm 23` → `PSA 23`; `Mzm 23–25` → `PSA 23-25`; `Mat. 5:3, 5-7` → `MAT 5:3,5-7`; `1 Kor 13` and `I Kor 13` and `1Kor13`… → `1CO 13`; `II Korintus 5:17` → `2CO 5:17`; `Yud 3` → `JUD 1:3`; `JHN 3:16` → `JHN 3:16` | As listed | Upper/lower case, extra spaces, tabs, non-breaking space, `—` as dash |
| TC-R-002 | Book table | All 66 books | Every book has a code, a name and an abbreviation; every alias and name is unique across books; every name and abbreviation parses to its own code | `Hak` vs `Hag`; `Yl`; `Am` |
| TC-R-003 | Errors | `""`, `Foo 1`, `Yoh`, `Yoh 3:0`, `Yoh 151`, `Yoh 3:21-16`, `Yoh 3:16,16`, `Yoh 3:16a`, `Yoh 3;4`, `Yoh 3.16` | `empty`, `unknown_book`, `missing_chapter`, `bad_number`, `bad_number`, `bad_range`, `bad_range`, `unsupported`, `unsupported`, `bad_number` | 100+ character input |
| TC-R-004 | Round trip | Every case of TC-R-001 | `ParseReference(r.String()).String() == r.String()` | — |
| TC-R-005 | Reading validation | Each rule in §3 | As specified | 20000 and 20001 characters; attribution 300/301; unknown translation code |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-R-001 | Create, read, edit, delete | Editor and team member | Team member can view only; the same passage typed two ways is one reading; `PATCH` with an old `version` → 409 `version_conflict` | — |
| IT-R-002 | Uniqueness | One reading | Same standard reference and translation → 409 `reading_exists` (with `reading_id`); another translation succeeds; another church's reading is invisible (404) | — |
| IT-R-003 | Lookup order | Fake provider returning `may_store = true`, then `false` | Stored reading first; then provider; `may_store = false` cannot be saved (422); `ErrNotAvailable` falls through to `reading: null` | — |
| IT-R-004 | Suggested attribution | Two readings in TB | The newer reading's attribution is suggested for TB, none for KJV | — |
| IT-R-005 | List and search | Readings in `id` and `zh-Hans` | Order by book, chapter, verse; substring search in reference and text; translation filter | — |
| IT-R-006 | Delete hook | `ReadingUsage` stub answering "in use" | 409 `reading_in_use`, nothing deleted | — |
| IT-R-007 | Constraints | Direct inserts bypassing the app | Duplicate (church, reference, translation), a reading in another church's name, an unknown translation are rejected on both dialects | — |

Web tests: unit tests for the reference preview (debounce, reason messages) and the "already exists" state; end-to-end test E2E-W-009 (add a reading by typing "Yoh 3:16-21", see it understood, save, find it by searching, add the same reference with different spelling and be told it exists); axe covers the new pages.

## 9. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| `invalid_reference` | 422, `reason` | `unknown_book`: "I don't know this book. Try the usual short name, e.g. Yoh or Mzm." `missing_chapter`: "Add a chapter, e.g. Yoh 3." `bad_number`: "Check the chapter and verse numbers." `bad_range`: "The verses must go in order, e.g. 16-21." `unsupported`: "This kind of reference isn't supported yet. Use one passage, e.g. Yoh 3:16-21." `empty`: "Type a reference." | Fix the input |
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
