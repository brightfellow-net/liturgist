# 09 — Planning Setup: Duties, Singing Parts, Templates, Services (Implementation)

> **Document type: Implementation.** Step 3 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), slice 3A.
> Status: decisions **[P-xx] Approved** 2026-10-03 (after the adversarial review; conditions in the [index](README.md#4b-proposed-decisions-step-3)). Code may be written from this document; the seeded words of 09 §3 stay a draft until Q-3.3 is closed. Items marked **[P-xx]** are decisions listed in the [index](README.md#4b-proposed-decisions-step-3). Nothing here is Approved until the owner says so.

## 1. Scope

The things a church sets up once so that liturgies can be made quickly: the **duties** people are assigned to, the **singing parts** that sing a section, **templates** (an ordered list of item titles), and **services** (a regular service with its language, default template and weekly times). Liturgies themselves are in [10](10-liturgy.md); their pages in [11](11-liturgy-editor.md).

| Left out | Why | Where it lands |
|---|---|---|
| Services in the setup wizard | The wizard (P-24) stays as built; a church adds services on the Services page, whose empty state explains how | Owner decides ([README Q-3.2](README.md#4c-questions-for-the-owner-step-3)) |
| The onboarding checklist on the church admin's home page | It also needs steps from steps 4–5 | Step 5 |
| Monthly patterns ("first Sunday") | SPEC §6 | After the MVP |
| Templates for other kinds of document, versions of templates | Not in the SPEC | Not planned |

## 2. Data model

Domain types in `domain/planning.go`; tables in [reference/schema.md](../reference/schema.md#step-3-tables). All four kinds are church-owned.

### 2.1 Duties and singing parts [P-55]

Both are short, ordered lists of names that the church can edit.

| Field | Duty | Singing part |
|---|---|---|
| `name` | 1–60 characters, trimmed, no line breaks | 1–40 characters, trimmed, no line breaks |
| `name_key` | `Fold(name)`; **unique per church** (409 `name_taken`, `reason`: `duty` / `singing_part`) | same |
| `position` | Dense 0..n-1; changed only by the reorder route | same |
| Limit per church | 50 | 30 |

- **Delete** is refused while the row is used: a duty by any liturgy item or assignment (409 `duty_in_use`), a singing part by any sequence entry (409 `singing_part_in_use`). Template items that name a duty are not "use": the delete clears their `default_duty_id` (the application nulls it in the same transaction, [schema](../reference/schema.md#step-3-tables)).
- **Rename** is allowed at any time. Liturgies refer to a duty by ID and show its **current** name (live, deliberately: a rename in the church's vocabulary should reach drafts at once). A published version freezes the names it shows in its own copy ([SPEC §5.5](../SPEC.md#55-review-workflow), step 5), so renaming never rewrites what was published. Decision [P-55]; owner decision Q-3.9 (2026-10-03). Step 5 must store the displayed duty names in the published version.
- Duties and singing parts carry no `version`: edits are single-field. The reorder route requires exactly the current set of IDs; two people who reorder the same list from the same starting point both succeed and the last write wins (the lists have at most 50 and 30 short names, the effect is visible at once, and nothing refers to a position). The whole rewrite of positions is **one transaction**, so no reader sees a half-moved list.
- **Limit reached** (the 51st duty, the 31st part, …): 422 `validation_failed` in the problem format of [01 §10](01-foundation.md#10-error-format-and-codes): `errors[0].location` names the field (`body.name`, `body.items`, `body.times`) and the problem carries `reason: "limit"`, `max` and `used` — the same shape for every fixed limit in this document (templates 50, template items 60, services 30, times 14). The count and the insert run in one `Tx.Write` under `LockChurch`, so a concurrent request that would be the 51st gets the same answer, never a 500.

### 2.2 Template

| Field | Rule |
|---|---|
| `name` | 1–100 characters; `name_key = Fold(name)`, unique per church (409 `name_taken`, `reason`: `template`) |
| `language` | `id`, `en`, `zh-Hans`, `zh-Hant`; defaults to the church's default content language |
| `items` | Ordered list, 0–60 entries (§2.3) |
| `version` | Starts at 1; +1 for every request that changes the template or its items |
| Limit per church | 50 templates |

### 2.3 Template item

| Field | Rule |
|---|---|
| `title` | 1–200 characters, trimmed |
| `item_type` | `song`, `reading`, `prayer`, `sermon`, `free_text`, `other` |
| `default_text` | 0–5000 characters (line endings stored as `\n`); only for `prayer`, `sermon`, `free_text`, `other`. Any text on a `song` or `reading` item → 422 |
| `default_duty_id` | Optional duty of this church; an unknown or foreign ID → 422 |

Template items have **no stable IDs and nothing refers to them**: liturgies copy them ([10 §3](10-liturgy.md#3-creating-a-liturgy)). `PATCH` therefore replaces the complete ordered list [P-57]; the server numbers positions 0..n-1.

### 2.4 Service

| Field | Rule |
|---|---|
| `name` | 1–100 characters; `name_key = Fold(name)`, unique per church (409 `name_taken`, `reason`: `service`) |
| `language` | As for templates; the language of liturgies made from this service |
| `default_template_id` | Optional template of this church. A service without one makes empty liturgies |
| `times` | 1–14 entries `{ weekday, time }`: `weekday` 1–7 (ISO: 1 = Monday, 7 = Sunday), `time` `HH:MM` 24-hour, 00:00–23:59; no two entries equal. The time is a wall-clock time in the church's time zone ([03 §10](03-identity-auth.md#10-first-time-setup): a zone `time.LoadLocation` accepts); a service never stores a UTC instant |
| `version` | As for templates |
| Limit per church | 30 services |

`times` is replaced as a whole list on `PATCH` (same reason as template items); omitting `times` keeps the list, and an empty list is 422 (`errors[0].location` `body.times`, `reason: "required"`): a service always has at least one time, so it always appears in "Prepare next week". Deleting a service keeps its liturgies, which hold a copy of the name ([10 §2.1](10-liturgy.md#21-liturgy)); `liturgies.service_id` becomes null.

**Deleting a template** that is the `default_template_id` of any service → 409 `template_in_use` with `service_ids`; liturgies made from it keep working (`template_id` becomes null).

## 3. Seeded defaults [P-56]

Created as ordinary editable rows in the church's **default content language** (`churches.default_language`), once per church:

| Language | Duties (in order) | Singing parts | Template "Ibadah Minggu" (items: type, default duty) |
|---|---|---|---|
| `id` | Liturgis, Pemandu Pujian, Pemusik, Pembaca Alkitab, Pengkhotbah, Multimedia, Kolektan | Semua, Pemandu, Jemaat, Pria, Wanita, Paduan Suara | Votum dan Salam (free text, Liturgis); Pujian (song, Pemandu Pujian); Pembacaan Alkitab (reading, Pembaca Alkitab); Doa Syafaat (prayer, Liturgis); Khotbah (sermon, Pengkhotbah); Persembahan (other, Kolektan); Berkat (free text, Liturgis) |
| `en` | Liturgist, Worship leader, Musician, Scripture reader, Preacher, Multimedia, Offering collector | All, Leader, Congregation, Men, Women, Choir | Votum and greeting; Praise; Scripture reading; Intercessory prayer; Sermon; Offering; Blessing (same types and duties) |
| `zh-Hans` | 主礼, 领唱, 乐手, 读经者, 讲道者, 多媒体, 司献 | 全体, 领唱, 会众, 男声, 女声, 诗班 | 主日崇拜: 宣召与问安; 赞美; 读经; 代祷; 讲道; 奉献; 祝福 |
| `zh-Hant` | 主禮, 領唱, 樂手, 讀經者, 講道者, 多媒體, 司獻 | 全體, 領唱, 會眾, 男聲, 女聲, 詩班 | 主日崇拜: 宣召與問安; 讚美; 讀經; 代禱; 講道; 奉獻; 祝福 |

Template names: `Ibadah Minggu` (id), `Sunday service` (en), `主日崇拜` (both Chinese). **All texts need review** — the order and the Chinese and English words are a draft, and the order must still be aligned with GKY Citragarden's real order of service ([SPEC §5.7](../SPEC.md#57-first-time-experience)) before the pilot ([Q-3.3](README.md#4c-questions-for-the-owner-step-3)). No default texts are seeded (votum words are often Bible text).

**When:** `Setup` seeds them in the same transaction that creates the church. For churches that already exist, `app.Seed` runs once at start-up after the migrations, per church, in **one** `Tx.Write`: take `LockChurch`; read the marker; if present, stop; insert the defaults; insert the marker `church_seeds(church_id, seed_key = 'step3')` **last**. Any error rolls back everything, so there is never a marker without its rows, and a church with the marker is skipped: a church that deleted a default never gets it back, and two servers starting together seed once (the lock, and the marker's primary key as a backstop). Seeding is the **only** writer of `church_seeds`. The marker is not a statement that all defaults exist today; nothing re-checks the rows. A marker made by hand without rows (an operator error) is repaired by deleting the marker row and restarting; this is the only recovery and is documented in the operations notes. This replaces the "migration seeds the defaults" wording of P-24 for rows that need new IDs and translated text.

## 4. API

All paths under `/api/v1`. **View duties and parts** = any member; **view templates and services** = a member holding `templates.edit` or `liturgy.edit` (anyone else: 403 `forbidden`, as for `/imports`); **edit** = `templates.edit` [P-55]. Responses carry `actions` ([04 §5](04-tenancy-extensions.md#5-authorization)): `{ edit, delete }`, true exactly when the member holds `templates.edit`. `actions` is **permission only** here: whether a duty, part or template is in use is a state that needs a query per row, so a row that cannot be deleted still shows `delete: true` and the 409 explains. This is a deliberate narrowing of the "scope and safeguards" wording of 04 §5, which is changed to say that safeguards are included only when they are known from the row already loaded.

| Method & path | Scope | Request | Response |
|---|---|---|---|
| `GET /duties`, `GET /singing-parts` | view | — | `{ items: [{ id, name, position, actions }] }` in position order |
| `POST /duties`, `POST /singing-parts` | edit | `{ name }` (added last) | 201 item; 422 `validation_failed` with `reason: "limit"` at the limit of §2.1 (these fixed limits are not entitlements, so never `limit_reached`) |
| `PATCH /duties/{id}`, `PATCH /singing-parts/{id}` | edit | `{ name }` | item |
| `PUT /duties/order`, `PUT /singing-parts/order` | edit | `{ ids: [...] }` — exactly the current IDs, once each | `{ items }`; any other list → 409 `version_conflict` (someone changed the list) |
| `DELETE /duties/{id}`, `DELETE /singing-parts/{id}` | edit | — | 204; 409 `duty_in_use` / `singing_part_in_use` |
| `GET /templates` | templates.edit or liturgy.edit | — | `{ items: [{ id, name, language, item_count, version, actions }] }` by `name_key` |
| `GET /templates/{id}` | same | — | Template with `items` and `version` |
| `POST /templates` | edit | `{ name, language?, items }` | 201 Template |
| `PATCH /templates/{id}` | edit | `{ version, name?, language?, items? }` | Template; stale `version` → 409 `version_conflict`, nothing written |
| `DELETE /templates/{id}` | edit | — | 204; 409 `template_in_use` |
| `GET /services`, `GET /services/{id}` | templates.edit or liturgy.edit | — | Service(s) with `times`, `default_template_id`, `default_template_name` |
| `POST /services` | edit | `{ name, language?, default_template_id?, times }` | 201 Service |
| `PATCH /services/{id}` | edit | `{ version, name?, language?, default_template_id?, times? }`; `default_template_id: ""` clears | Service; stale → 409 `version_conflict` |
| `DELETE /services/{id}` | edit | — | 204 |

Rules: every write is one `Tx.Write` under `LockChurch` where it checks a limit or a uniqueness rule that the database cannot express alone ([02 §2.1](02-persistence.md#21-atomic-operations)); `name_key` uniqueness is also a unique index, and the unique-violation maps to 409 `name_taken`. `PATCH` semantics follow [04 §6](04-tenancy-extensions.md#6-step-1-church-and-member-api) (omitted = unchanged). As built (slice 3A): operation IDs are `listDuties`, `createDuty`, `reorderDuties`, `renameDuty`, `deleteDuty` (and the same for singing parts), `listTemplates`, `getTemplate`, `createTemplate`, `updateTemplate`, `deleteTemplate` and the same five for services. Deleting a duty or a singing part closes the gap in the positions, so they stay dense. A seed that fails at start-up is logged as `seeding the defaults failed` and the server still starts, without the defaults for that church; the next start-up tries again.

Logging: info lines `duty_created`, `template_updated`, `service_deleted`, … with actor and IDs; template item text is never logged.

## 5. Pages

Menu item **Liturgies** for every member who holds `liturgy.edit`, `liturgy.comment`, `liturgy.approve`, `liturgy.manage` or `templates.edit` (team members without those scopes do not see it). Its frame has tabs shown per scope: **Liturgies** ([11](11-liturgy-editor.md)), **Templates**, **Services**, **Duties**, **Singing parts** (the last three and Templates for `templates.edit` and `liturgy.edit`; members without `templates.edit` see them read-only). All text through i18n ([05 §6](05-web-shell.md#6-translations)); tap targets and text size as in [05 §7](05-web-shell.md#7-accessibility-and-text-size).

| Route | Page |
|---|---|
| `/liturgies/templates` | Template list with item counts; empty state "No templates yet" with "Add a template" |
| `/liturgies/templates/new`, `/liturgies/templates/{id}` | Form: name, language, then the **items editor**: one card per item with title, kind, default duty and default text (text only for the kinds that take it), and **Move up / Move down / Remove** buttons with text, as for song sections ([06 §4](06-song-library.md#4-pages), P-52); "Add an item". "Delete template" asks first. After a `version_conflict` the page offers "Reload" and keeps the typed input |
| `/liturgies/services` | Service list: name, language, times in words ("Sunday 07:00; Wednesday 19:00"), default template; "Add a service". Empty state explains what a service is and offers "Add a service" |
| `/liturgies/services/new`, `/liturgies/services/{id}` | Form: name, language, default template (a list), then the **times** editor: weekday (list) and time (`<input type="time">`) per row; "Add a time"; Remove buttons with text |
| `/liturgies/duties`, `/liturgies/singing-parts` | One list each: inline rename, Move up / Move down, Delete (with the 409 message), and "Add" |

**As built (slice 3A).** The menu item and the four tabs exist; `/liturgies` redirects to Templates until slice 3C adds the liturgy list as the first tab. Template and service forms return to their list after saving. The forms wait for the duty and template lists before they render, so a select never starts empty and saves a default away. A service in the list shows its template as "Template: …". Members who may see templates and services but not edit them get a read-only page. Add buttons follow the `templates.edit` scope (an empty list has no row to carry `actions`); every other button follows `actions`. Messages exist in English and Indonesian; the Indonesian wording is a draft for the owner's review.

## 6. Ports

| Port | Methods | Implemented by | Used by |
|---|---|---|---|
| `ChurchStore.Duties()`, `.SingingParts()`, `.Templates()`, `.Services()` | create, update, delete, `ByID`, `List`, `Count`, `Reorder`; templates and services: `Replace` of items / times with a conditional `version` update | `adapters/sqlstore` | Planning use cases |
| `ChurchStore.Seeds()` | `Applied(ctx, key) (bool, error)`, `Mark(ctx, key, at)` — both only inside the one `Tx.Write` of §3, with `Mark` last | `adapters/sqlstore` | `app.Seed`, `Setup` |

Scoped-repository rules ([04 §5](04-tenancy-extensions.md#5-authorization)) apply: every method takes the church from the scope, never from an argument. `Entitlements` is not consulted in slice 3A (no limits apply to setup data).

## 7. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Let a liturgy refer to a template item | Copy title, type, text and duty into the liturgy | SPEC §5.2: editing a template must never change a liturgy |
| Hard-code duty or singing-part names in code | Seeded rows the church can edit; code refers to IDs | SPEC §5.7: names are per church and per language |
| Check a role name for template or service rights | Check the `templates.edit` scope | SPEC §4 rule 5 |
| Delete a duty or singing part that is in use and "fix" the references | Refuse with 409 `duty_in_use` / `singing_part_in_use` | Liturgies would silently lose who sings or does what |
| Seed defaults in a migration with fixed text | `app.Seed` with the church's language, guarded by the marker | New IDs and translated text are needed per church; the marker stops re-seeding what a church deleted |
| Store the service time in UTC | `HH:MM` in the church's time zone | A service is "Sunday 07:00" whatever the clock-change rules; there are none in Indonesia, but self-hosters elsewhere |
| Number weekdays 0–6 in one place and 1–7 in another | ISO 1–7 everywhere (API, database, web) | One convention; Sunday is 7, never 0 |
| Merge two duties' names that differ only by case or accents | `name_key = Fold(name)` unique | "Pemusik" and "pemusik" would be two duties on one printed list |
| Replace template items by editing rows one at a time from the client | One `PATCH` with the complete list in one transaction | A half-saved template would give liturgies a wrong order |

## 8. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-P-001 | Duty and part names | 1, 60/61 and 40/41 character names; names differing by case or accents | Accepted / rejected as in §2.1; the folded key is equal for `Pemusik`, `pemusik`, `Pémusik` | Empty, only spaces, line break inside, full-width letters |
| TC-P-002 | Template validation | Items with each type; text on a `song` item; 60 and 61 items; foreign duty ID | Rules of §2.3 | `\r\n` text becomes `\n`; item title of 200/201 characters |
| TC-P-003 | Service times | `{1,"07:00"}`, `{7,"7:00"}`, `{8,"07:00"}`, `{3,"24:00"}`, duplicates, 14 and 15 entries | Only the first is valid | `00:00`, `23:59`; same time on two weekdays is valid |
| TC-P-007 | Fixed limits and empty times | The 51st duty and 31st part through two parallel requests; a service `PATCH` with `times: []`, with `times` omitted | One 422 `validation_failed` with `reason: "limit"`, `max`, `used` for the loser, never a 500; `[]` → 422 `required`; omitted keeps the list | Time zone not loadable → rejected where it is saved |
| TC-P-004 | Reorder | Complete list, list missing one ID, list with a foreign ID, duplicate ID | Positions 0..n-1 in the new order; the others → `version_conflict` | Empty list on an empty table |
| TC-P-005 | Seeding | Each of the four languages | The tables of §3, positions dense, template items reference the seeded duties | Second run changes nothing; church with the marker but no rows stays empty |
| TC-P-006 | Delete rules | Duty used by an assignment, by a liturgy item, by a template item only; singing part used by an entry | 409 / 409 / deleted with the template item's duty cleared / 409 | — |

### Integration tests (HTTP through `httptest`; repository contract tests run on both dialects)

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-P-001 | Permissions | Church admin, editor (no `templates.edit`), liturgist, team member | Team member: duties 200, templates 403; editor with only `liturgy.edit`: reads templates, `POST` 403; church admin writes; another church's IDs → 404 | — |
| IT-P-002 | Version conflict | One template, two sessions | Second `PATCH` with the old `version` → 409, data unchanged; replacing items keeps positions dense | — |
| IT-P-003 | Uniqueness and limits | 50 duties, two names equal after folding, two churches with the same name | 51st → 422 `validation_failed`; second equal name → `name_taken`; the other church is unaffected; two concurrent creates of the same name give one 201 and one 409 (race harness, [02 §2.1](02-persistence.md#21-atomic-operations)) | — |
| IT-P-004 | Seeding on start-up | A database with two churches and no marker; then a restart | Both churches seeded once, in their own language; the restart adds nothing; two servers started together seed once | — |
| IT-P-005 | Template and service deletion | Service with a default template | Template delete → 409 `template_in_use` with `service_ids`; after the service is deleted, template delete succeeds | — |
| IT-P-006 | Seeding is atomic | A start-up seed that fails after the template (injected error); a second server starting at the same time | No rows and no marker after the failure (retry seeds fully); two concurrent starts seed once; a marker without rows is skipped (documented repair) | — |

Web tests: unit tests for the items editor (reordering, removing, text only for the kinds that take it, `version_conflict` message) and the times editor; end-to-end test E2E-W-012 (add a duty, a singing part, a template with three items and a service; the lists show them; a conflict message appears when a second session saved first); axe covers the new pages (E2E-W-005 is extended).

## 9. Error handling matrix

| Error | Detection | User message (en) | Recovery |
|---|---|---|---|
| `validation_failed` | 422 with field list | "Check this field." on the fields | Fix and resubmit |
| `name_taken` | 409, `reason` | "There is already a duty / singing part / template / service with this name." | Choose another name |
| `version_conflict` | 409 | "This was changed by someone else. Reload to see their version." | Reload button; unsaved input kept |
| `duty_in_use` | 409 | "This duty is used in a liturgy, so it can't be deleted." | Stay on page |
| `singing_part_in_use` | 409 | "This singing part is used in a liturgy, so it can't be deleted." | Stay on page |
| `template_in_use` | 409 | "This template is the default of a service. Change the service first." | Stay on page |
| `validation_failed` at a count limit | 422 | "You can have at most {{max}} of these." | Delete one first |
| `forbidden` | 403 | "You don't have permission to do this." | Stay on page |
| `not_found` | 404 | "Not found." | Back to the list |

## 10. References

| Topic | Location |
|---|---|
| Templates, services, duties, first-time experience | [SPEC.md §5.1](../SPEC.md#51-liturgy-templates), [§5.2](../SPEC.md#52-weekly-liturgy), [§5.7](../SPEC.md#57-first-time-experience), [§11](../SPEC.md#11-decisions-log) |
| Tables | [reference/schema.md](../reference/schema.md#step-3-tables) |
| Atomic operations, locks | [02 §2.1](02-persistence.md#21-atomic-operations), [02 §3](02-persistence.md#3-connections-and-transactions) |
| Authorization and `actions` | [04 §5](04-tenancy-extensions.md#5-authorization) |
| Scopes | [03 §8](03-identity-auth.md#8-member-roles-and-permissions) |
| Folding | [06 §5.1](06-song-library.md#51-folding) |
| Web shell, accessibility | [05](05-web-shell.md) |
