# Liturgist App — MVP Specification

> Handoff document for Claude Code. Read this fully before writing code.
> Where this document says **Open decision**, ask the project owner before choosing.

## 1. Context

- **Project:** Brightfellow (brightfellow.net), a community that builds open-source tools for small churches.
- **GitHub org:** github.com/brightfellow-net
- **License:** Apache-2.0
- **Distribution model:** free, self-hostable software first; later a paid hosted SaaS on brightfellow.net subdomains for churches that don't want to self-host.
- **Pilot church:** GKY Citragarden (Indonesia).
- **Owner:** Hutomo, professional software engineer (C/C++ background, comfortable with low-level systems work).

## 2. The problem

GKY Citragarden prepares its weekly liturgy entirely by hand:

1. The **liturgist** chooses next week's liturgy structure and the team members who will run it, then picks song titles and Bible readings for each liturgy item, and delegates to the administrator.
2. The **administrator** drafts the official liturgy document: manually searches lyrics for every song, looks up the text for every reading, and copy-pastes everything into the document.
3. The administrator sends the draft to the liturgist for review. Corrections go back and forth until the liturgist approves and saves it as final.
4. The final document is printed and distributed to each liturgy team member.
5. The liturgist then delegates to the **multimedia team**, who rebuild the same content as on-screen presentation slides.

The same information is re-handled three times (liturgist → document → slides). Most errors and review rounds come from manual copying.

## 3. Product idea

**Enter it once, as structured data, and generate everything else from it.**

The liturgist builds the liturgy directly in the app. Songs and readings are picked from a reusable church library, so lyrics and verse text are entered once and reused forever. The official document is rendered from the data, reviewed inside the app, and published to the team.

## 4. Users and roles (MVP)

| Role | Can do |
|---|---|
| **Liturgist** | Create/edit liturgies, assign team, pick songs/readings, review, comment, approve, publish |
| **Administrator** | Edit liturgies, maintain song and reading library, fix content, respond to review comments |
| **Team member** | View all published liturgies of their church (read-only) |
| **Church admin** | Manage users, roles, templates, church settings |

One user may hold several roles. Multimedia team is a team-member role for now; dedicated slide features come after the MVP.

## 5. MVP scope

### 5.1 Liturgy templates
- A church defines reusable templates: an ordered list of liturgy items (e.g. Votum & Salam, Pujian, Pembacaan Alkitab, Doa Syafaat, Khotbah, Persembahan, Berkat).
- Each template item has: title, item type (song, reading, prayer, sermon, free text, other), optional default text, optional default role.
- Multiple templates per church (regular Sunday, Holy Communion, special services).

### 5.2 Weekly liturgy
- Create a liturgy for a date and service from a template; items are copied in and can then be added, removed, reordered, or edited freely without affecting the template.
- Support more than one service on the same date.
- For each item, attach content depending on type:
  - **Song:** link to a song in the library, choose which sections/verses are sung and in what order.
  - **Reading:** Bible reference (book, chapter, verse range) linked to a stored reading text.
  - **Other:** free text.
- Assign team members to roles for this liturgy (liturgist, worship leader, musicians, readers, multimedia, etc.). Role names are configurable per church.

### 5.3 Song library
- Per-church library. **The app ships with no lyrics.** Each church enters its own.
- Song fields: title, alternative titles, hymnal source and number (e.g. KJ 1, PKJ 12, NKB 5), author/composer, default key, copyright/license notes.
- Lyrics are stored **as ordered sections** (verse 1, verse 2, chorus, bridge…), not one text block. This is required so slides can be generated later.
- Search by title, hymnal number, and lyric text.
- Usage history: which liturgies used this song (useful for planning and future license reporting).

### 5.4 Readings
- **The app ships with no Bible text.** Modern Indonesian translations (e.g. LAI Terjemahan Baru) are copyrighted.
- MVP approach: user enters a reference and pastes the text once; the app stores it keyed by reference + translation and reuses it next time the same reference is chosen.
- **Reference parser.** Users type references the way Indonesian churches write them, e.g. "Yoh 3:16-21", "Kej. 1:1–2:3", "Mzm 23", with Indonesian book names and common abbreviations (Kej, Kel, Im, Bil, Ul, … Mat, Mrk, Luk, Yoh, Kis, Rm, …). The parser stores them in a standard form using standard book codes (e.g. `JHN 3:16-21`), so the same passage is recognised however it was typed and any provider can look it up. English and Mandarin book names come later. Different verse numbering between translations is allowed for in the design but not handled in the MVP.
- **Default translation.** Each church sets a default translation (TB for the pilot), used when a reading is added; it can be changed per reading.
- **Attribution.** Each stored reading carries an attribution line (e.g. the copyright notice required by the publisher), shown on the published view and in the PDF.
- Keep the text source pluggable (`BibleTextProvider`, see 8.2) so a licensed API, imported Bible files or public-domain translations can be added later. Each provider result states its source, its attribution line, and whether the text may be stored.

### 5.5 Review workflow
States and transitions:

```
Draft ──submit──▶ In Review ──approve──▶ Approved ──publish──▶ Published
                     │
                     └──request changes──▶ Needs Revision ──resubmit──▶ In Review
```

- Liturgist and administrator can comment on a **specific liturgy item**, not just the liturgy as a whole. Comments can be resolved.
- Approved and Published liturgies are locked. Editing after approval requires explicitly reopening it (back to Draft), and this should be recorded.
- Keep a simple history of state changes (who, when).

### 5.6 Outputs
- **Printable PDF** of the official liturgy document, rendered from the data.
- **Mobile-friendly web view** of a published liturgy, shareable by link, plus a "my assignments" view for each team member.
- **Open decision:** layout of the PDF. Get GKY Citragarden's current liturgy document as the reference design.

## 6. Out of scope for MVP (but design for it)

- Presentation slides / export to PowerPoint, Google Slides, OpenLP, or a built-in presenter. *(Keep lyrics sectioned and content separate from formatting so this is just another renderer.)*
- WhatsApp or email notifications. *(A copyable text summary of the published liturgy is a cheap MVP stand-in if time allows.)*
- Multilingual parallel text (e.g. Indonesian + Mandarin/English side by side). *(Avoid assumptions that each item has exactly one language.)*
- Hosted SaaS operations: billing, plans, signup, church onboarding. *(Only the entitlement stub in 8.2 is in MVP scope.)* *(The tenancy and URL structure in section 8.1 is in scope from the start, so the SaaS needs no rework later.)*
- Volunteer rostering, availability, rotation.
- Lectionary integration.

## 7. Data model sketch

Starting point, not final. Refine as needed.

- **Church**: id, slug (unique, URL-safe), name, default_translation, settings
- **ChurchSlugRedirect**: old_slug, church_id *(only needed if slug renaming is allowed; see 8.1)*
- **User**: id, name, email?, phone?, password_hash *(platform-wide, no church_id; at least one of email or phone; each is unique across the platform)*
- **Membership**: id, user_id, church_id, roles *(roles apply per church; a user may belong to several churches)*
- **RoleType**: id, church_id, name (e.g. "Pemandu Pujian", "Pemusik")
- **Template**: id, church_id, name
- **TemplateItem**: id, template_id, position, title, item_type, default_text, default_role_type_id
- **Liturgy**: id, church_id, date, service_name, template_id (origin), state, archived_at (null = active), archived_by, created_by, timestamps
- **LiturgyItem**: id, liturgy_id, position, title, item_type, song_id?, song_section_order?, reading_id?, text?
- **Assignment**: id, liturgy_id, user_id (or free-text name for non-users), role_type_id
- **Song**: id, church_id, title, alt_titles, hymnal_source, hymnal_number, author, default_key, license_notes
- **SongSection**: id, song_id, position, label (Verse 1, Chorus…), text
- **Reading**: id, church_id, reference (standard form, e.g. `JHN 3:16-21`), reference_display (as typed, e.g. "Yoh 3:16-21"), translation, text, attribution, source_provider *(unique per church + standard reference + translation)*
- **Comment**: id, liturgy_id, liturgy_item_id?, author_id, body, resolved, timestamps
- **StateChange**: id, liturgy_id, from_state, to_state, user_id, timestamp
- **PublishedVersion**: id, liturgy_id, version, content (complete copy of the liturgy as published: items, chosen song sections with text, reading text, assignments), published_at, published_by

## 8. Non-functional requirements

- **Easy self-hosting is a top priority.** Small churches rarely have IT staff. Target a single binary or a single `docker run` command and simple backup (copy one file or run one command).
- **Database:** support SQLite and PostgreSQL behind one data-access layer.
  - *SQLite is the self-host default:* no separate database server to install or maintain, the whole database is one file, and backup is copying that file. The expected load (a few editors, a few dozen viewers per church) is well within its limits.
  - *PostgreSQL is the expected choice for the hosted SaaS:* better concurrent writes across many churches, and mature backup/replication tooling.
  - Migrations must run on both. Tests should run against both, or at minimum against SQLite with PostgreSQL in CI.
  - The SaaS uses one shared PostgreSQL database (decided 2026-10-02; see decisions log).
- **Tenant-ready:** all data scoped by `church_id`. See 8.1.

### 8.1 Tenancy and URL structure

The hosted SaaS serves every church from one product subdomain, with the church identified by the first path segment:

```
https://liturgist.brightfellow.net/<church_slug>/...
e.g. https://liturgist.brightfellow.net/gky-citragarden/liturgies/2026-10-11
```

**Deployment modes** (set by configuration, same code path):

| Mode | URL shape | Use |
|---|---|---|
| `single` | `/...` (no slug) | Self-hosted install for one church. The tenant is fixed by config; slug routing is disabled. |
| `multi` | `/<church_slug>/...` | Hosted SaaS, or a self-hoster serving several churches. |

**Requirements:**

1. **Church slug.** Each church has a unique, lowercase, URL-safe slug (`a-z`, `0-9`, `-`; e.g. `gky-citragarden`). Validate on creation.
2. **Reserved slugs.** Reject slugs that collide with system routes or could confuse users, at minimum: `login`, `logout`, `signup`, `api`, `admin`, `static`, `assets`, `health`, `docs`, `about`, `help`, `www`. Keep the list in one place and check it against the router's top-level routes in a test.
3. **Tenant resolution middleware.** In `multi` mode, a middleware reads the first path segment, loads the church, and stores it as the request's tenant context. Unknown slug → 404. Handlers never read the slug themselves.
4. **Scoped data access.** All queries for church-owned data go through the tenant context, so a handler cannot fetch another church's records by accident (e.g. a repository/query layer that requires the tenant, rather than ad-hoc `WHERE church_id = ?` in handlers).
5. **Authorization checks membership.** All churches share one domain, so the session cookie is valid on every church's path. On every request to a church path, verify the logged-in user is a member of *that* church with the needed role; otherwise 403 (or 404 to avoid revealing which churches exist). **This is the most important security requirement of the tenancy design** and must have dedicated tests (user of church A requesting church B's liturgy, comments, PDF, share links, and API endpoints).
6. **URL generation through a helper.** All internal links, redirects, PDF links, and share links are built by a helper that applies the current mode and slug. No hardcoded paths in templates or handlers.
7. **Share links.** Published-liturgy links (web view, PDF, "my assignments") are normal church URLs and require login; after login the user returns to the link they opened. Guest links without login are deferred (see decisions log).
8. **Slug changes.** Prefer slugs to be immutable for MVP. If renaming is supported, store the old slug in `ChurchSlugRedirect` and 301-redirect old URLs, because links get printed and shared in WhatsApp groups.
9. **Platform-level routes** (login, signup, platform admin) live outside any church prefix. After login, send the user to their church, or a church picker if they belong to several.
- **UI language:** Indonesian first, with i18n from the start (English as second locale).
- **Mobile-friendly:** team members will mostly open links on phones.
- **No bundled copyrighted content** (lyrics, Bible text).
- Apache-2.0 license headers/notice per repository convention.
- Tests for workflow state transitions and permissions at minimum.

### 8.2 Extensibility and editions

**Business model guiding the design.** Every feature of the core liturgy workflow is open source and free: templates, liturgies, song library, readings, review workflow, PDF, sharing. The hosted SaaS earns money from hosting and convenience, plus features that carry real running or license costs (e.g. WhatsApp Business API notifications, licensed Bible text, managed backups, custom domains, email delivery, media storage, priority support). Do not gate core workflow features behind a paywall.

**Extension points, not dynamic plugins.** The core defines interfaces for the parts likely to vary. Implementations are compiled in; there is no dynamic plugin loading and no promised public plugin API in the MVP.

| Extension point | Responsibility | Community implementation (MVP) | Possible premium / later implementations |
|---|---|---|---|
| `Notifier` | Tell team members a liturgy was published or changed | Copyable text summary (and optionally email if configured) | WhatsApp Business API, managed email |
| `BibleTextProvider` | Return text for a reference + translation | Manual entry with reuse (5.4) | Licensed provider API, public-domain translations |
| `Exporter` | Render a liturgy to an output format | PDF, web view | Slides (PPTX/OpenLP/presenter), other print layouts |
| `Storage` | Store generated files and future media | Local filesystem | Object storage (S3-compatible) |
| `AuthProvider` | Authenticate users | Per open decision 2 | SSO / Google login |

Rules:
- Core code depends only on the interfaces, never on a concrete implementation.
- Implementations register themselves in one place at startup (a registry or explicit wiring), chosen by configuration.
- Community implementations live in the open repository. Premium implementations live in a separate private module and are compiled into the SaaS build only; the open repository must build, run and pass tests without it.
- Self-hosters and contributors can add implementations through the same interfaces.

**Edition vs entitlement** are separate concepts:
- *Edition* = which implementations are compiled into a build (community build vs SaaS build).
- *Entitlement* = which features a specific church may use on the SaaS, based on its plan.

Entitlement requirements:
- One central check, e.g. `entitlements.has(church, feature) -> bool`. Feature code never inspects plans or prices directly.
- Feature names are constants defined in one place.
- **MVP:** implement the check as a stub that always returns `true`, and call it at the extension points that will become premium (e.g. before using a non-default `Notifier`). Plans, billing and plan-to-feature mapping come with the SaaS work.
- Self-hosted installs: every compiled-in feature is enabled; no license keys or activation.

#### 8.2.1 Usage limits (SaaS free plan)

Entitlements cover numeric limits as well as on/off features:

- `entitlements.has(church, feature) -> bool`
- `entitlements.limit(church, limit_name) -> int | unlimited`

Limit names are constants defined in one place, next to feature names. Self-hosted installs always get `unlimited`.

**MVP:** the stub returns `unlimited` for every limit, but the checks below are already called at the right places, so enabling a plan later is configuration only. Archiving and deleting unpublished liturgies are part of the MVP for everyone, since they are useful even without limits.

**Planned free-plan limits** (values live in plan configuration, not code):

| Limit name | Free plan | Meaning |
|---|---|---|
| `max_active_liturgies` | 7 | Number of non-archived liturgies a church can have at once |
| `max_unpublished_liturgies` | 4 | Number of liturgies not yet published (a subset of active liturgies) |
| `max_team_members` | 12 | Number of active memberships in the church (any role) plus pending, unexpired invites |

**Rules for `max_active_liturgies`:**

1. **What counts.** Every liturgy that is not archived counts, in any state (Draft, In Review, Needs Revision, Approved, Published), past or upcoming, and whether it was created from a template or from scratch. Each service counts separately when a date has several.
2. **Archiving frees a slot.** Users with the liturgist or church admin role can archive a **Published** liturgy. Archiving is a manual action; the app never archives anything automatically. Archived liturgies are kept in full, are never deleted by the limit, and **remain viewable read-only on every plan**, including PDF export. Archiving manages slots and clutter; it does not take a church's history away.
3. **Deleting unpublished liturgies frees a slot.** Users with the liturgist or church admin role can delete any liturgy that is not yet Published (Draft, In Review, Needs Revision, Approved), with a confirmation step. Deletion removes the liturgy with its items, assignments and comments. Published liturgies cannot be deleted, only archived.
4. **Creation is blocked at the limit.** Creating a new liturgy (from a template, from scratch, or by duplicating) when the church is at the limit is blocked with a message offering the ways forward: upgrade, archive a published liturgy, or delete an unpublished one. The message should list the oldest published liturgies with a one-click archive action, so a church can continue in seconds.
5. **Restoring needs a slot.** Unarchiving a liturgy makes it count again, so it is only allowed when the church is under the limit.
6. **Existing work is never blocked.** Editing, reviewing, approving, publishing, viewing, and exporting liturgies that already exist are never affected by the limit; only creating new ones is.
7. **Downgrades are gentle.** If a church on a paid plan drops to free while over the limit, nothing is archived or locked automatically. The church simply cannot create new liturgies until it archives enough to get under the limit.
8. **Visibility.** Church settings and the liturgy list show usage (e.g. "6 of 7 active liturgies"), with a warning when one slot is left, so nobody discovers the limit on Saturday night.

**Rules for `max_unpublished_liturgies`:**

1. **What counts.** Every non-archived liturgy in an unpublished state: Draft, In Review, Needs Revision, or Approved. Counting all unpublished states (not only Draft) prevents working around the limit by moving liturgies into review.
2. **Both limits apply.** An unpublished liturgy counts toward both `max_unpublished_liturgies` and `max_active_liturgies`. Creating a liturgy requires a free slot in both.
3. **Freeing a slot.** Publishing a liturgy frees an unpublished slot (it still counts as active). Deleting an unpublished liturgy frees a slot in both limits.
4. **Blocked creation message** explains which limit was reached. When it is the unpublished limit, list the unpublished liturgies with their state so the user can finish or delete one.
5. **Why 4:** a church with one weekly service can plan about a month ahead; a church with several services per Sunday can prepare the next Sunday fully plus some upcoming ones. It sits below the active limit of 7, leaving room for at least 3 published liturgies to stay active for reference.
6. Downgrades follow the same gentle rule as above: nothing is deleted or locked; creation is blocked until the church is under the limit.

**Rules for `max_team_members`:**

1. **What counts.** Every active membership in the church, whatever its roles (liturgist, administrator, church admin, team member), plus every pending, unexpired invite. Free-text names used in assignments are not counted and stay unlimited. A person who belongs to several churches counts once in each.
2. **Invites reserve a slot.** Creating an invite takes a slot, so accepting it never fails. Expired or cancelled invites free their slot.
3. Enforced at the moment of adding: inviting or adding a member beyond the limit is blocked with a clear upgrade message.
4. **Freeing a slot.** A church admin can remove a member or cancel an invite; the slot is freed immediately. "Remove member" is part of the MVP.
5. Existing members are never removed or locked out automatically, including after a downgrade; new invites are simply blocked until the count is under the limit.
6. Church settings show usage (e.g. "9 of 12 team members").

**Tests:** counting across all states and multiple services per date; blocked creation via every creation path (template, scratch, duplicate) for each limit separately; archive only allowed for Published and only by permitted roles; delete only allowed for unpublished liturgies and only by permitted roles; archived liturgies viewable and exportable on the free plan; unarchive blocked at the limit; existing liturgies fully usable at and over the limit; downgrade behavior; blocked member invites; team-member count includes every role and pending invites but not free-text assignees; accepting a reserved invite never fails; expired, cancelled or removed entries free a slot.

## 9. Open decisions (ask the owner)

1. ~~**Tech stack** (language, web framework, frontend approach).~~ *Resolved 2026-10-02; see section 11.*
2. ~~**Authentication** for MVP (email + password, magic link, or invite-only accounts created by church admin).~~ *Resolved 2026-10-02; see section 11.*
3. **PDF layout** — based on GKY Citragarden's current document.
4. **Bible text source** beyond manual entry. *Partly decided 2026-10-02 (see section 11): MVP scope and post-MVP import/downloads are settled; licensed TB/TB2 waits on LAI's reply about licensing.*
5. ~~**Repository name** under github.com/brightfellow-net.~~ *Resolved 2026-10-02; see section 11.*
6. ~~**SaaS database layout:** one shared PostgreSQL database with `church_id` on every table (simplest operations, recommended starting point) vs. one SQLite file per church (strong isolation, but harder migrations and cross-church tooling).~~ *Resolved 2026-10-02; see section 11.*
7. **Free-plan limit values:** confirm 7 active liturgies, 4 unpublished liturgies, and 12 team members with the pilot church (e.g. how many services they hold per Sunday and how far ahead they plan). Not needed for MVP.
8. ~~**What counts as a team member** for `max_team_members`: user accounts only, or also free-text names used in assignments for people without accounts.~~ *Resolved 2026-10-02; see section 11.*

## 10. Suggested build order

1. Project skeleton, data model, migrations (SQLite + PostgreSQL), auth, church + users + roles, tenancy middleware and URL helper with membership tests (8.1), extension-point interfaces and entitlement stub with feature and limit checks (8.2, 8.2.1).
2. Song library (CRUD, sections, search) and readings store.
3. Templates and weekly liturgy editor (items, reorder, songs, readings, assignments).
4. Review workflow with item-level comments and state history.
5. Published web view, "my assignments" view, PDF output.
6. Self-host packaging (single binary / Docker), backup instructions, README.

Pilot with GKY Citragarden after step 5; gather feedback from the liturgist, administrator, and multimedia team before starting slides.

## 11. Decisions log

Record resolved decisions here (date, decision, reason). Move items from section 9 here once settled.

| Date | Decision | Reason |
|---|---|---|
| 2026-10-02 | SaaS uses path-based tenancy: `liturgist.brightfellow.net/<church_slug>/` | One DNS record and one TLS certificate; simpler than per-church subdomains |
| 2026-10-02 | Support SQLite (self-host default) and PostgreSQL (expected for SaaS) | Zero-maintenance self-hosting; PostgreSQL suits a shared hosted service |
| 2026-10-02 | Core workflow stays free and open; SaaS charges for hosting, convenience, and features with real running/license costs | Fits the community identity and Apache-2.0 license; a feature paywall would be easy to bypass and erode contributor trust |
| 2026-10-02 | Extension points with compiled-in implementations; no dynamic plugin system in MVP; premium code in a private module | Most of a plugin system's benefit without designing a stable public plugin API too early |
| 2026-10-02 | Separate edition (build contents) from entitlement (per-church plan); MVP ships an always-true entitlement stub | Keeps paywall logic out of feature code; pricing changes become configuration |
| 2026-10-02 | No premium licenses for self-hosters for now | License keys and enforcement cost more than they would earn at this scale |
| 2026-10-02 | SaaS free plan uses usage limits (planned: 7 active liturgies, 12 team members), enforced through `entitlements.limit()`; self-host unlimited | Gives a natural upgrade path while hosting costs justify limits on the hosted free tier |
| 2026-10-02 | Liturgy limit counts all non-archived liturgies; churches free a slot by manually archiving a published liturgy or by upgrading; nothing is archived, deleted, or locked automatically | Owner's intended model; keeps the church in control and never blocks work on existing liturgies |
| 2026-10-02 | Archived liturgies stay viewable read-only (including PDF) on every plan | Archiving manages slots, not access; churches keep their history |
| 2026-10-02 | Unpublished liturgies can be deleted by liturgist/church admin; published ones can only be archived | Lets churches free slots from abandoned drafts while keeping published history |
| 2026-10-02 | Free plan also limits unpublished liturgies (planned: 4), counting Draft, In Review, Needs Revision and Approved | Caps work-in-progress; counting all unpublished states prevents bypassing via review |
| 2026-10-02 | Clean architecture: domain → application (use cases + ports) → adapters, wired in one composition root by configuration. Persistence, search, transactions and the 8.2 extension points are ports; repositories are tenant-scoped | Core logic stays independent of the database and framework, so SQLite and PostgreSQL can be swapped without touching it; tenant-scoped ports enforce 8.1 rule 4 |
| 2026-10-02 | One shared SQL persistence adapter with a small dialect layer (placeholders, upserts, locking, full-text search), not separate SQLite and PostgreSQL adapters. Separate migration folders per database with matching versions | Each query is written once and the two databases can't silently drift apart; database-specific differences stay in one small place |
| 2026-10-02 | Use-case tests run against the real SQLite adapter (in-memory), not hand-written fakes. Repository contract tests run against both SQLite and PostgreSQL (PostgreSQL in CI) | Fast enough for every test run; avoids keeping a third, fake persistence implementation; meets the "tests against both" requirement in 8 |
| 2026-10-02 | Tech stack: Go backend exposing a JSON API (the HTTP adapter over the use cases) + React SPA (Vite + TypeScript) for all web pages, built into the Go binary with `go:embed`. A React Native (Expo) mobile app may follow later, for team members and the multimedia team | Owner has 5 years of Go and knows React. One frontend approach on every page. The multimedia presenter needs client-side, offline-capable code, and React skills and TypeScript packages carry over to React Native. Self-hosters still get a single binary |
| 2026-10-02 | Backend libraries: chi router; Huma (code-first OpenAPI 3.1); `database/sql` + `sqlx` with hand-written SQL; `modernc.org/sqlite` (pure Go, no CGO) and `pgx`; goose migrations (embedded); ULID IDs; `log/slog`; `go-i18n`; testcontainers-go for PostgreSQL in CI; `depguard` to enforce layer boundaries | Small, stable dependencies; static single binary; OpenAPI generated from Go types keeps the TypeScript client in sync |
| 2026-10-02 | Frontend libraries: `openapi-typescript` + `openapi-fetch` (types and client generated from the OpenAPI spec); TanStack Query; React Router; React Hook Form + Zod; Tailwind + shadcn/ui; dnd-kit; i18next (same message files as Go, `id` default, `en` second); `vite-plugin-pwa`; Vitest + Testing Library; Playwright for end-to-end | Common, maintained choices. API client, types and i18n can be shared with a future React Native app. shadcn/ui code lives in the repo, which limits dependency churn |
| 2026-10-02 | Sessions are server-side and stored in the database, in an HttpOnly SameSite=Lax cookie with CSRF protection; a mobile app later uses bearer tokens through `AuthProvider`. The login method itself is still open decision 2 | Sessions can be revoked and work the same on SQLite and PostgreSQL |
| 2026-10-02 | URL layout: in `multi` mode the API is at `/<church_slug>/api/v1/...` and SPA routes at `/<church_slug>/...` (the server returns `index.html`); in `single` mode `/api/v1/...` and `/...`. Platform routes (login, platform API) sit outside any church prefix | Keeps 8.1 unchanged: the first path segment is the church, so the tenant middleware and URL helper work for both API and pages |
| 2026-10-02 | The API returns the actions the current user may take on each resource (e.g. `"actions": {"submit": true, "edit": false}`). The UI shows or hides controls from these and never re-implements permission or workflow rules | All rules stay in Go, in one place; web and mobile clients stay consistent |
| 2026-10-02 | Undo/redo in the liturgy editor is server-side: each editor change is a use-case command recorded per liturgy with the data to reverse it. History is shared by everyone editing that liturgy and lasts after reload, while the liturgy is editable (Draft, Needs Revision). The UI may apply undo immediately and confirm with the server in the background | History is the same on every device and for both editors; it can also show what changed since the last review |
| 2026-10-02 | PDF: print stylesheet on the published view for the MVP; server-side Typst later, shipped alongside the binary (in the Docker image or release archive). PDF does not need to be inside the single binary | Server cost is negligible, and server output is the same on every device and shareable by URL; the final layout waits for open decision 3 |
| 2026-10-02 | Repository layout: one monorepo with the Go module at the root (`cmd/`, `internal/{domain,app,adapters}`, `migrations/{sqlite,postgres}`) and pnpm workspaces for TypeScript (`web/`, `packages/api-client`, `packages/i18n`). `make build` builds the frontend, generates API types, then runs `go build`; CI fails if generated types are out of date | One place for the whole web product. How a future mobile app shares the API client and translations is decided later, with the mobile app |
| 2026-10-02 | Auth for MVP: invite-only accounts with a password. A church admin creates an invite (link expires in 7 days) and shares it through any channel; the recipient sets a password. No open sign-up into a church. Passwords: argon2id, minimum 10 characters, checked against a list of commonly leaked passwords, no composition rules, rate-limited login attempts. Google login, passkeys, email magic links and two-step login for admins come later through `AuthProvider` | Works without email setup, which matters for self-hosters; invite links can be shared on WhatsApp; churches control who joins |
| 2026-10-02 | Login identifier is email or phone number; each user has at least one and may have both, and each is unique across the platform. Emails are stored trimmed and lowercased; phone numbers in international format, with Indonesia as the default region (`0812…` → `+62812…`). One "Email or phone number" field on the login form | Many volunteers use WhatsApp more than email; the identifier needs no verification because an admin invited the person |
| 2026-10-02 | Password reset: by email link when the user has an email and the install has email configured; otherwise a church admin generates a reset link to share; a `liturgist user reset-password` command for locked-out admins | Every install can recover accounts, with or without email |
| 2026-10-02 | Every team member has a full account, including read-only team members | Owner's choice: one consistent way to access the app |
| 2026-10-02 | Users are platform-wide (`User` has no `church_id`); church access and roles live in `Membership`. Inviting an email or phone that already belongs to a user adds a membership after they log in, and never creates a duplicate account | Supports users in several churches (8.1 rule 9); in `single` mode everyone simply has one membership |
| 2026-10-02 | First-time setup: a one-time setup link printed to the log on first start opens a "create church and first admin" page; a `liturgist setup` command does the same for scripts and Docker. Both call the same use case | Friendly for non-technical installers, and scriptable for automated installs |
| 2026-10-02 | Share links require login (8.1 rule 7): the published view, its PDF and "my assignments" are only shown to logged-in members of that church, and after login the user returns to the link they opened. Guest links are deferred; if the pilot asks for them, they will be per-liturgy tokens (stored hashed, read-only, expiring a week after the service by default, revocable) | Lyrics and Bible text are copyrighted, and church licences usually cover the congregation, not anyone holding a link; one access rule keeps the 8.1 rule 5 tests sufficient |
| 2026-10-02 | Every member of a church can view all of that church's published liturgies, not only those they are assigned to | Simpler, and lets musicians and readers look ahead |
| 2026-10-02 | Publishing stores a `PublishedVersion`: a complete copy of the liturgy as published. The published view and PDF always read from the latest version, never from the editable liturgy. When a published liturgy is reopened, members keep seeing the last published version with a "being revised" notice; edits stay invisible until the next publish. Archiving keeps all versions | The team always has something to rehearse from; later library edits don't change what was distributed; exact record for future licence reporting (5.3) |
| 2026-10-02 | Sessions last 90 days, extended whenever the app is used; configurable per install | Team members rarely need to log in again on their phones |
| 2026-10-02 | SaaS database layout (open decision 6): one shared PostgreSQL database with `church_id` on every church-owned row. Indexes on church-owned tables start with `church_id`; foreign keys between church-owned tables include `church_id`; deleting a church cascades from `Church`. Not one SQLite file per church, and not one schema per church | Users and memberships are platform-wide, so per-church files would still need a separate platform database; one migration run, managed backups and simple cross-church reporting; expected scale is easily handled by one PostgreSQL instance |
| 2026-10-02 | PostgreSQL row-level security as a second line of defence: the hook is built now (the PostgreSQL dialect sets the current church at the start of each transaction through the transaction port); policies on church-owned tables are switched on before the SaaS launch. Platform-level queries (login, membership lookup) use a separate path allowed to bypass them. SQLite has no equivalent and relies on tenant-scoped repositories | Catches a buggy query in our own code before it can return another church's data; building the hook now is cheap and avoids retrofitting |
| 2026-10-02 | Export one church to a SQLite file and import it again (`liturgist church export <slug>` / import), built after the MVP and before the SaaS launch, reusing the shared SQL adapter | Gives back the main advantage of per-church files: a church can take its data to self-hosting, or be restored on its own; also useful for self-hosters in `multi` mode |
| 2026-10-02 | `max_team_members` (open decision 8) counts every active membership in the church, whatever its roles, plus pending unexpired invites, which reserve a slot so accepting never fails. Free-text assignees are not counted. A person in several churches counts once in each. A church admin frees a slot by removing a member or cancelling an invite; "remove member" is in the MVP | Clear and easy to explain ("12 people can use the app"). Free-text names are for guests and would otherwise block edits to existing liturgies; people without accounts can't log in, so using them as a workaround gains little |
| 2026-10-02 | Main translation for the pilot is LAI Terjemahan Baru (TB), copyrighted by LAI. The owner will contact LAI about licensing (church-use rules, a licence for the hosted service, storing text, required attribution, TB2, cost) | TB is what GKY Citragarden uses; a licence from LAI (directly or through an API that carries TB) is the only clean way to provide TB text in the app |
| 2026-10-02 | Bible text in the MVP: manual entry with reuse (5.4), plus a reference parser that understands Indonesian book names and abbreviations (e.g. "Yoh 3:16-21", "Kej. 1:1–2:3", "Mzm 23") and stores references in a standard form (standard book codes, e.g. `JHN 3:16-21`). Each church has a default translation. Stored readings carry an attribution line, shown on the published view and PDF. Different verse numbering between translations is allowed for in the design but not handled in the MVP | Manual entry needs no licence from the app; standard references let any later provider look text up; most licences require attribution |
| 2026-10-02 | Bible text after the MVP: an import provider for Bible files the church has rights to (USFM, OSIS or Zefania XML), and an optional download of public-domain and open-licence texts (e.g. Chinese Union Version 1919, KJV, WEB, BSB; AYT if its licence is confirmed). Nothing is bundled in the binary, so "ships no Bible text" stays true. Licensed TB/TB2 for the SaaS depends on LAI's answer. Copying text from websites is not allowed | Fills whole Bibles in one step for self-hosters and gives Mandarin and English quickly, without distributing copyrighted text |
| 2026-10-02 | Each `BibleTextProvider` result states its source, its attribution line, and whether the text may be stored; stored copies (in `Reading` and `PublishedVersion`) record which provider they came from | API and publisher licences often limit storing text and require attribution; recording the source keeps each stored copy traceable to its licence |
| 2026-10-02 | Repository (open decision 5): `brightfellow-net/liturgist` (Go module `github.com/brightfellow-net/liturgist`, binary `liturgist`, Docker image `ghcr.io/brightfellow-net/liturgist`). The private SaaS build lives in `brightfellow-net/liturgist-saas` (premium implementations, the SaaS `main`, deployment config). A future mobile app lives in a separate repository; its name and how it shares code are decided later | Matches the product name, the SaaS URL and the CLI commands in this spec; the org name already says Brightfellow, so no prefix is needed |