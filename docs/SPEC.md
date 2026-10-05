# Liturgist App — MVP Specification

> **Document type: Strategic** (what to build and why). How to build each step is in the implementation documents under [`docs/impl/`](impl/README.md); see [section 12](#12-references).
> Read this fully before writing code. Where this document says **Open decision**, ask the project owner before choosing.
> Sections 1–10 are authoritative for **what** to build. Approved implementation documents (`docs/impl/`, `docs/reference/`) decide **how**, and win on implementation details. The decisions log (section 11) records when and why each decision was made; where a log row and a section differ, the section wins. If two current documents conflict, stop and ask the owner ([precedence rule](impl/README.md#2-how-to-use-these-documents)).

## 1. Context

- **Project:** Brightfellow (brightfellow.net), a community that builds open-source tools for small churches.
- **GitHub org:** github.com/brightfellow-net
- **License:** Apache-2.0
- **Distribution model:** free, self-hostable software first; later a paid hosted SaaS on brightfellow.net subdomains for churches that don't want to self-host.
- **Pilot church:** GKY Citragarden (Indonesia).
- **Owner:** Hutomo, professional software engineer.

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

What a member may do is decided by **roles**, which each church defines itself. A role has a name, a short description and a set of **scopes**. Scopes are fixed in the code; each is one capability. A member can hold several roles.

**Baseline (every member, no role needed):** view all published liturgies of the church, "my assignments", and their own profile. A member with no role is a **team member**.

**Scopes:**

| Scope | Allows |
|---|---|
| `church.settings` | Change church settings |
| `members.view` | See the member list with contact details |
| `members.manage` | Invite and remove members; create password-reset links |
| `roles.manage` | Create, edit and delete roles; assign roles to members |
| `library.edit` | Maintain songs and readings; imports |
| `templates.edit` | Maintain templates and regular services |
| `liturgy.edit` | Create and edit liturgies, assign the team, submit and resubmit for review |
| `liturgy.comment` | Comment on liturgy items; resolve comments |
| `liturgy.approve` | Approve, request changes, publish, reopen |
| `liturgy.manage` | Archive and unarchive published liturgies; delete unpublished liturgies |

**Ready-made roles**, created for every new church; names, descriptions and scopes can be changed:

| Role | Scopes | Matches today's job at GKY |
|---|---|---|
| Church admin | All ten scopes | Whoever manages the app for the church; holding every scope lets them give every role (safeguard 2) |
| Liturgist | `members.view`, `templates.edit`, `liturgy.edit`, `liturgy.comment`, `liturgy.approve`, `liturgy.manage` | The liturgist |
| Editor | `members.view`, `library.edit`, `liturgy.edit`, `liturgy.comment` | The administrator who drafts the document |

**Safeguards:**
1. **No lock-out:** at least one member must always hold both `roles.manage` and `members.manage`; any change that would break this is refused.
2. **No privilege escalation:** a member can only put scopes they hold themselves into a role, and can only assign a role to someone if they hold every scope in it.
3. Deleting a role that members hold needs confirmation and removes it from them, subject to rule 1.
4. Scopes stay coarse, so new features usually fall under an existing scope. When a release adds a scope, a migration adds it to the ready-made roles (found by a hidden origin key even if renamed), and the release notes tell admins to check their own roles.
5. Code checks scopes only, never role names.

**Duties** (e.g. Pemandu Pujian, Pemusik, Pembaca Alkitab) are a separate concept: the tasks people are assigned in a liturgy (5.2). They are not roles and grant no permissions.

The multimedia team are team members for now; dedicated slide features come after the MVP.

## 5. MVP scope

### 5.1 Liturgy templates
- A church defines reusable templates: an ordered list of liturgy items (e.g. Votum & Salam, Pujian, Pembacaan Alkitab, Doa Syafaat, Khotbah, Persembahan, Berkat).
- Each template item has: title, item type (song, reading, prayer, sermon, free text, other), optional default text, optional default duty.
- Multiple templates per church (regular Sunday, Holy Communion, special services).
- Each template has a **language** (e.g. a Mandarin template has Mandarin item titles), defaulting to the church's default language.

### 5.2 Weekly liturgy
- Create a liturgy for a date and service from a template; items are copied in and can then be added, removed, reordered, or edited freely without affecting the template.
- Each liturgy has a **language**: the service's main language (e.g. `id` for the Indonesian service, `zh-Hans` for the Mandarin service), taken from the template. Content is entered in that language; parallel text comes later (see 6).
- Support more than one service on the same date.
- **Services.** A church defines its regular services once: name, language, default template, and one or more weekly times (any weekday, e.g. "Ibadah Umum 1: Sunday 07:00", "Persekutuan Doa: Wednesday 19:00", "Ibadah Pemuda: Saturday 17:00 and Sunday 15:00").
- **Prepare next week.** The user picks a week; the app lists every scheduled service occurrence in it with date and time, all ticked by default. Unticking skips one; one click creates a liturgy for each, from the service's template and in its language.
- **One-off services** (e.g. Christmas Eve, Good Friday) are created individually with any name, date and template. A liturgy keeps a copy of its service name, so renaming a service doesn't change past liturgies.
- For each item, attach content depending on type:
  - **Song:** link one or more songs from the library; several songs in one item form a medley, sung in order. For each song, build its **sequence**: which sections are sung and in what order, with repeats allowed (e.g. V1, Chorus, V2, Chorus, Chorus). Choosing a song fills the sequence from the song's default arrangement, or all verses in order if it has none; the liturgist then edits it freely.
    - Each entry in the sequence can name **who sings it** (e.g. Semua, Pemandu, Jemaat, Pria, Wanita, Paduan Suara), chosen from a list each church configures.
    - Each song in the item has a **key** (defaulting to the song's default key), and any entry can carry a **key change** (e.g. up a half step before the last chorus). Keys are shown as "Do = G" by default; a church setting can switch to "G".
    - Optional free-text notes on each song in the item (e.g. "tempo lambat") and on each entry (e.g. "2x").
    - Non-lyric entries (intro, instrumental interlude, spoken lines) are not in the MVP; the model leaves room for them.
  - **Reading:** Bible reference (book, chapter, verse range) linked to a stored reading text.
  - **Other:** free text.
- Assign team members to **duties** for this liturgy (e.g. liturgist, worship leader, musicians, readers, multimedia). Duty names are configurable per church. Duties are not roles and grant no permissions (see 4).
- **Several people editing at once.** The liturgy and each item carry a version; every change states the version it was based on. The server rejects a change only when that same item (or, for reordering and state changes, the liturgy) changed in the meantime; the editor then shows "changed meanwhile", reloads that item and keeps the user's own text so it can be re-applied. Other people's changes appear when the page is reloaded or when a save conflicts. *(Live updates and the "Budi is also editing" indicator are on the icebox since 2026-10-03; see the decisions log.)*
- **Undo/redo** is per person: Ctrl+Z undoes your own latest change, and is refused with a clear message if someone has since changed that item. The history is stored per liturgy and visible to everyone editing it.

### 5.3 Song library
- Per-church library. **The app ships with no lyrics.** Each church enters its own.
- Song fields: title, alternative titles, hymnal source and number (e.g. KJ 1, PKJ 12, NKB 5), lyricist, composer, translator, default key, copyright fields (below), and an optional **default arrangement** (the usual order of sections, e.g. V1, PC, C, V2, PC, C, B, C, C).
- Lyrics are stored **as ordered sections** (verse 1, verse 2, chorus, bridge…), not one text block. This is required so slides can be generated later. Each section has a kind (verse, pre-chorus, chorus, bridge, tag, intro, ending, other) and, for verses, a number.
- A section cannot be deleted while an unpublished liturgy uses it. Published liturgies are unaffected because they keep their own copy (`PublishedVersion`).
- Each song has a **language**. The same hymn in another language is a separate song (its own hymnal number, sections, verse count and licence notes), linked to its other-language versions through a song group, so they can be found together and paired up for parallel text later.
- Search by title, hymnal number, and lyric text. Chinese text is searched by substring matching (Chinese has no spaces between words, so full-text search can't find words inside a line); Indonesian and English use full-text search. Both sit behind the search interface.
- Usage history: which liturgies used this song (useful for planning and license reporting).
- **Copyright information.** The app can't grant permission; it helps a church record what it knows, show required credit lines, and report usage. Translations of public-domain hymns are usually copyrighted works of their own, so original authors and translator are recorded separately.
  - Song fields: copyright holder (e.g. Yamuger), copyright line (the exact text to print, e.g. "© Yayasan Musik Gereja Indonesia"), CCLI song number, licence status (unknown, public domain, covered by church licence, permission obtained), and free-text licence notes.
  - The song library can be filtered by licence status, so a church can review its gaps.
  - **No enforcement:** publishing and printing are never blocked because of licence status.
  - **Usage report** (MVP): a CSV export of songs used in a chosen period, with title, hymnal number, CCLI number, how often, and in which services (e.g. for CCLI reporting).

**Importing existing content.** An empty library is the biggest hurdle to adoption, so getting a church's existing lyrics in must be quick:
- **Paste and split** (MVP, an everyday tool): the user pastes lyrics; the app splits them at blank lines and recognises section labels in Indonesian, English and Chinese ("1.", "Bait 1", "Ayat 1" → verse 1; "Reff", "Refrein", "Chorus", "副歌" → chorus; "Bridge", "Pre-Chorus", "Interlude"), suggesting an unlabelled block repeated after each verse as the chorus. The user checks and adjusts the preview before saving.
- **File formats**, each an `Importer` implementation (8.2):
  - MVP: OpenLyrics XML (OpenLP) and ChordPro (`.cho`).
  - For the pilot: EasyWorship 6/7 song databases (believed to be SQLite with lyrics stored as RTF; the exact version is to be confirmed with the pilot church), then PowerPoint (`.pptx`) for songs that only exist as slides. Until the EasyWorship importer exists, the free OpenLP's EasyWorship importer plus OpenLyrics export is the fallback route.
  - Later: Word (`.docx`) liturgy documents, spreadsheets (song metadata), other presentation software (e.g. ProPresenter).
- **Review step for file imports:** an import creates a batch of candidates. The admin accepts, edits, merges with an existing song, or skips each one (or accepts all). Duplicates are detected by hymnal source and number (e.g. KJ 1) or by normalised title.
- Accepted candidates are saved through the normal use cases, so validation, church scoping and limits apply exactly as when typing songs in. Imported readings get their translation's attribution.
- Imported content is the church's own material in its own install, consistent with "ships no lyrics".

### 5.4 Readings
- **The app ships with no Bible text.** Modern Indonesian translations (e.g. LAI Terjemahan Baru) are copyrighted.
- MVP approach: user enters a reference and pastes the text once; the app stores it keyed by reference + translation and reuses it next time the same reference is chosen.
- **Reference parser.** Users type references the way Indonesian churches write them, e.g. "Yoh 3:16-21", "Kej. 1:1–2:3", "Mzm 23", with Indonesian book names and common abbreviations (Kej, Kel, Im, Bil, Ul, … Mat, Mrk, Luk, Yoh, Kis, Rm, …). The parser stores them in a standard form using standard book codes (e.g. `JHN 3:16-21`), so the same passage is recognised however it was typed and any provider can look it up. English and Mandarin book names come later. Different verse numbering between translations is allowed for in the design but not handled in the MVP.
- **Translations** are records with a code, name and language (e.g. `TB` → `id`, `CUV` → `zh-Hans`), so a reading's language follows from its translation.
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

- Members with `liturgy.comment` can comment on a **specific liturgy item**, not just the liturgy as a whole. Comments can be resolved.
- Only **Draft** and **Needs Revision** liturgies can be edited. **In Review** is read-only apart from comments; to change anything, the reviewer requests changes (→ Needs Revision).
- Approved and Published liturgies are locked. Editing after approval requires explicitly reopening it (back to Draft), and this should be recorded.
- Keep a simple history of state changes (who, when).

### 5.6 Outputs
- **Printable PDF** of the official liturgy document, rendered from the data.
- **Print formats** (MVP):
  - **Variants:** the **team version** (the official liturgy document: every item, who leads it, lyrics with sequences and singing parts, reading text, keys and notes, the assignments list) and the **musician sheet** (one compact page: each song with hymnal number, key and key changes, sequence, singing parts and notes; no full lyrics, or first lines only).
  - **Paper sizes:** A4 and F4 / Folio (215 × 330 mm).
  - **Options:** lyrics in full or first lines only; reading text on or off; assignments, keys and notes shown or hidden; normal or large text. The church sets defaults; the person printing can change them.
  - **Header:** church name, optional church logo (uploaded in settings, stored through `Storage`), service, date and time.
  - **Credits:** each song's copyright line under the song in the team version and the published view (on by default; a church setting can turn it off; not on the musician sheet), and an optional licence footer with the church's licence numbers (e.g. "CCLI License #1234567").
  - **Production:** a print view of the published liturgy, styled for print, printed or saved as PDF through the browser. Song headings are kept with their first section, and sections are not split across pages where possible. Server-side PDF (Typst) comes later, for booklets and an exact match with the reference layout.
- **Mobile-friendly web view** of a published liturgy, shareable by link, plus a "my assignments" view for each team member.
- **Reading mode** (MVP) on the published view, for reading or leading from a phone during the service: large text in one column, high contrast, menus hidden; the viewer's own items highlighted, with a "Go to my part" button; a keep-screen-on toggle (browser Wake Lock API); works offline through the PWA cache.
- **Open decision:** layout of the PDF. Get GKY Citragarden's current liturgy document as the reference design.

- **WhatsApp messages** (MVP; no WhatsApp API, no cost, no setup). After publishing, the app offers ready-made texts:
  1. **Team summary** for the WhatsApp group, with **Copy** and **Share to WhatsApp** (`https://wa.me/?text=…`, which opens WhatsApp's chat picker with the text filled in).
  2. **Personal messages**: a list of everyone assigned, each with **Send via WhatsApp** (`https://wa.me/<phone>?text=…`, which opens a chat with that person with the text filled in; the sender presses send). People without a phone number get a Copy button; free-text assignees are listed without a button.
  3. **Change summary** after republishing: compares with the previous `PublishedVersion` and lists what changed (songs, readings, assignments).
  - **Never include lyrics or Bible text**: only titles, hymnal numbers, keys, reading references and assignments, plus the link to the full liturgy (which needs login).
  - Checkboxes to include or leave out songs, keys and readings.
  - WhatsApp formatting only: `*bold*` and plain URLs (no Markdown links); short lines that read well on a phone.
  - Tone of personal messages: a friendly greeting with the person's name, avoiding "kamu" or "Bapak/Ibu".
  - Messages are written in the **liturgy's language** when the app has a translation for it (Indonesian and English in the MVP, Mandarin later), otherwise in the church's default UI language. So an Indonesian service gets Indonesian messages even when the sender uses the app in English. Message templates each church can edit come later.

  Example team summary:

  ```
  *Liturgi Ibadah Umum 1*
  Minggu, 11 Oktober 2026 · 07:00

  *Petugas*
  Liturgis: Andreas
  Pemandu Pujian: Budi
  Pemusik: Clara, Daniel

  *Lagu*
  1. KJ 1 · Haleluya, Pujilah · Do = G

  *Bacaan:* Yoh 3:16-21

  Liturgi lengkap:
  https://…
  ```

  Example personal message:

  ```
  Shalom Budi 🙏
  Mengingatkan tugas pelayanan:
  *Pemandu Pujian* · Ibadah Umum 1
  Minggu, 11 Oktober 2026 · 07:00

  Liturgi lengkap:
  https://…
  Terima kasih atas pelayanannya!
  ```

### 5.7 First-time experience
- **Setup wizard** (the setup page from the decisions log): church name and first admin (name, email or phone, password); default UI language for members (English pre-selected); default content language; default Bible translation; time zone (WIB, WITA or WIT); key display ("Do = G" or "G"). Regular services are added afterwards on the Services page (decisions log, 2026-10-03). Everything can be changed later in settings.
- **Seeded defaults**, created as normal editable data in the church's default language:
  - role types (e.g. Liturgis, Pemandu Pujian, Pemusik, Pembaca Alkitab, Pengkhotbah, Multimedia, Kolektan);
  - singing parts (Semua, Pemandu, Jemaat, Pria, Wanita, Paduan Suara);
  - translation names only, no text (e.g. TB, TB2, BIS, CUV, KJV, WEB);
  - a starter template "Ibadah Minggu" with item titles only and no default texts (votum words are often Bible text, which can't be shipped); a Mandarin version if Mandarin is chosen. The order is to be aligned with GKY Citragarden's real order of service.
- **Feedback link:** an optional "Kirim masukan" (send feedback) link in the menu, pointing to a URL set in settings (e.g. a WhatsApp group or a form); hidden when not set.
- **Onboarding checklist** on the church admin's dashboard until done or dismissed: invite your team; import your songs; check your template; create next week's liturgies.
- **Helpful empty screens:** every empty list explains what to do next with direct actions (e.g. an empty song library offers EasyWorship import, paste lyrics, add a song).
- **Team member welcome:** after accepting an invite, team members land on "Tugas saya" (my assignments) with a short welcome and simple steps to add the app to the home screen on Android and iPhone.
- **No sample data** in installs (almost all Indonesian hymn lyrics are copyrighted).
- **Defaults for features added later:** when a release introduces seeded defaults, its migration also adds them to existing churches that don't have them yet, so churches set up early are not left without them.

### 5.8 Accessibility and older volunteers
Many readers, elders and liturgists are older, use mid-range Android phones with large system text, and sometimes read from a phone at the lectern.
- **Text size:** the UI uses relative sizes (no fixed pixel sizes) and works with the phone's text size at 200%. An in-app control (A · A+ · A++) on the published view and "Tugas saya" is saved to the person's account, so it follows them to a new phone.
- **Simpler navigation** for people who are only team members (no roles): just "Tugas saya" (my assignments) and "Liturgi" (published liturgies); no admin menus or empty editor screens.
- **General rules:** tap targets of at least 48 px; buttons labelled with words, not icons alone; plain wording in every language (e.g. Indonesian "Tambahkan ke layar utama", English "Add to home screen"; never "PWA" or "sync"); dark mode follows the phone's setting.
- Reading mode is described in 5.6.

## 6. Out of scope for MVP (but design for it)

- Presentation slides / export to PowerPoint, Google Slides, OpenLP, or a built-in presenter. *(Keep lyrics sectioned and content separate from formatting so this is just another renderer.)*
- Automatic WhatsApp or email notifications and reminders (e.g. the day before), and message templates each church can edit. *(The MVP has copyable messages and wa.me links instead; see 5.6. Automatic WhatsApp sending is a SaaS `Notifier` using the WhatsApp Business API.)*
- Multilingual parallel text (e.g. Indonesian + Mandarin/English side by side) and pinyin under Chinese lyrics. *(Avoid assumptions that each item has exactly one language. The MVP model leaves room: liturgies, templates and songs carry a language; songs in different languages are linked through a song group; readings refer to a translation with a language. Parallel text later adds secondary-language text alongside the existing single-language content. Until then, bilingual singing uses a medley of the linked language versions.)*
- Hosted SaaS operations: billing, plans, signup, church onboarding. *(Only the entitlement stub in 8.2 is in MVP scope.)* *(The community tenancy foundation in 8.1 is in scope from the start, so the SaaS needs no rework later; multi-church hosting itself (8.1.1) is SaaS-only.)*
- Volunteer rostering, availability, rotation.
- Lectionary integration.
- AI-assisted import (a language model turning messy documents into structured songs). *(Later, opt-in only: bring-your-own API key, or a SaaS service used only with the church's explicit consent, because it sends church content to an outside service. It would be another `Importer`.)*
- Importing past liturgies as history. *(Not planned; songs matter far more than old services.)*
- Monthly service patterns (e.g. Holy Communion on the first Sunday). *(MVP: weekly times only; create these as one-off services or pick a different template when preparing that week.)*
- A public demo site run by Brightfellow, so people can try the app before installing.
- More print variants: a **congregation version** (lyrics and responses, no team names, keys or notes; printing lyrics for the congregation also depends on the church's licences) and **"my part"** (only what one person reads or says).
- **A5 booklets** (A4 folded, pages reordered for folding). *(Needs server-side PDF: Typst plus a booklet step, e.g. `pdfcpu`, to be checked. Until then, churches can use their printer driver's booklet option.)*

## 7. Data model sketch

Starting point, not final. Refine as needed.

- **Church**: id, name, default_language (content), default_ui_language (`en` or `id`), default_translation_id, time_zone (e.g. `Asia/Jakarta`), logo_file? (stored through `Storage`), settings (incl. key display format: "Do = G" or "G", print defaults, church licences such as a CCLI licence number, and whether credit lines are shown) *(slug is SaaS-only; see 8.1.1)*
- **ChurchSlugRedirect**: old_slug, church_id *(SaaS-only, and only if slug renaming is allowed; see 8.1.1)*
- **User**: id, name, email?, phone?, password_hash, preferences (text size, UI language), last_seen_at *(platform-wide, no church_id; at least one of email or phone; each is unique across the platform)*
- **Membership**: id, user_id, church_id *(roles via `MembershipRole`; roles apply per church; a user may belong to several churches)*
- **Role**: id, church_id, name, description, origin? (`church_admin`, `liturgist`, `editor` for ready-made roles; null for roles the church creates)
- **RoleScope**: role_id, scope *(scopes are constants in code; see 4)*
- **MembershipRole**: membership_id, role_id
- **Duty**: id, church_id, name (e.g. "Pemandu Pujian", "Pemusik") *(liturgy tasks; formerly `RoleType`)*
- **Template**: id, church_id, name, language
- **Service**: id, church_id, name, language, default_template_id
- **ServiceTime**: id, service_id, weekday, time *(one or more per service)*
- **TemplateItem**: id, template_id, position, title, item_type, default_text, default_duty_id
- **Liturgy**: id, church_id, date, service_id? (null for one-off services), service_name (copy), time?, language, template_id (origin), state, version, archived_at (null = active), archived_by, created_by, timestamps
- **LiturgyItem**: id, liturgy_id, position, title, item_type, reading_id?, text?, version *(songs are attached through `LiturgyItemSong`)*
- **LiturgyItemSong**: id, liturgy_item_id, position, song_id, key?, note? *(several per item = medley)*
- **SequenceEntry**: id, liturgy_item_song_id, position, kind (MVP: `section` only; later `instrumental`, `spoken`, …), song_section_id?, singing_part_id?, key_change?, note?
- **Assignment**: id, liturgy_id, user_id (or free-text name for non-users), duty_id
- **Song**: id, church_id, song_group_id?, language, title, alt_titles, hymnal_source, hymnal_number, lyricist, composer, translator, default_key, copyright_holder, copyright_line, ccli_song_number?, licence_status (unknown, public_domain, church_licence, permission_obtained), license_notes, default_arrangement? (ordered list of section ids)
- **SongSection**: id, song_id, position, kind (verse, pre_chorus, chorus, bridge, tag, intro, ending, other), number?, label (Verse 1, Chorus…), text
- **SingingPart**: id, church_id, name (e.g. Semua, Pemandu, Jemaat, Pria, Wanita, Paduan Suara; seeded with defaults, editable per church)
- **ImportBatch**: id, church_id, source_format (e.g. paste, openlyrics, chordpro, easyworship, pptx), created_by, created_at, status
- **ImportCandidate**: id, batch_id, kind (song, reading), parsed_data, duplicate_of_id?, decision (pending, accept, merge, skip)
- **Reading**: id, church_id, reference (standard form, e.g. `JHN 3:16-21`), reference_display (as typed, e.g. "Yoh 3:16-21"), translation_id, text, attribution, source_provider *(unique per church + standard reference + translation)*
- **Translation**: id, code (e.g. `TB`, `CUV`), name, language *(language codes are BCP 47: `id`, `zh-Hans`, `zh-Hant`, `en`)*
- **SongGroup**: id, church_id, name? *(links the language versions of the same song)*
- **Comment**: id, liturgy_id, liturgy_item_id?, author_id, body, resolved, timestamps
- **StateChange**: id, liturgy_id, from_state, to_state, user_id, timestamp
- **LiturgyEdit**: id, liturgy_id, user_id, command (e.g. MoveItem, EditItemText, SetSequence), reverse_data (what is needed to undo it), created_at, undone_at? *(the per-liturgy undo/redo history)*
- **PublishedVersion**: id, liturgy_id, version, content (complete copy of the liturgy as published: items, chosen song sections with text, reading text, assignments, song credit lines and the licence footer as they were at publishing), published_at, published_by

## 8. Non-functional requirements

- **Easy self-hosting is a top priority.** Small churches rarely have IT staff. Target a single binary or a single `docker run` command and simple backup (copy one file or run one command).
- **Database:** support SQLite and PostgreSQL behind one data-access layer.
  - *SQLite is the self-host default:* no separate database server to install or maintain, the whole database is one file, and backup is copying that file. The expected load (a few editors, a few dozen viewers per church) is well within its limits.
  - *PostgreSQL is the expected choice for the hosted SaaS:* better concurrent writes across many churches, and mature backup/replication tooling.
  - Migrations must run on both. Tests should run against both, or at minimum against SQLite with PostgreSQL in CI.
  - The SaaS uses one shared PostgreSQL database (decided 2026-10-02; see decisions log).
- **Tenant-ready:** all church-owned data scoped by `church_id`, behind a tenant context and church-scoped repositories (8.1). One church per community install; multi-church hosting is SaaS-only (8.1.1).
- **UI language:** English first, Indonesian second, with i18n from the start. English is the source language for all UI text; every Indonesian message file must contain every key. Each church has a default UI language for its members (English unless changed); each user can choose their own, saved in `User.preferences`. This is separate from content language (5.2), which stays per church, liturgy and song.
- **Mobile-friendly:** team members will mostly open links on phones.
- **Privacy:** no analytics or tracking scripts in any install. A short privacy notice page in the app explains what personal data is stored (names, phone numbers, emails; IP addresses only briefly, to block password guessing, deleted within an hour), why, and who can see it; the church admin can add the church's contact details. It is linked from the login and invite pages. Applies to every install (Indonesia's UU PDP).
- **Accessibility:** WCAG 2.2 AA as the target, checked automatically with axe in the Playwright tests (see 5.8).
- **No bundled copyrighted content** (lyrics, Bible text).
- Apache-2.0 license headers/notice per repository convention.
- Tests for workflow state transitions and permissions at minimum.

### 8.1 Tenancy

Each community install serves **exactly one church**. To serve several churches, run several instances. Hosting many churches in one install (the SaaS) is built in `liturgist-saas` on top of the foundation below (see 8.1.1), so it needs no changes to the schema or the use cases.

**Community foundation (this repository):**

1. **Church-owned data.** Every church-owned row has `church_id`; indexes and foreign keys between church-owned tables include it.
2. **Tenant context.** Every request carries the church it is for, provided by the `TenantResolver` port. The community implementation returns the install's only church from the database (created by setup); the server refuses to start if the database holds more than one church. Handlers never decide the church themselves.
3. **Scoped data access.** All queries for church-owned data go through the tenant context, so a handler cannot fetch another church's records by accident (e.g. a repository/query layer that requires the tenant, rather than ad-hoc `WHERE church_id = ?` in handlers).
4. **Authorization checks membership.** Users are platform-wide (see 7), so on every request the use cases verify that the logged-in user is a member of the request's church with the needed role; otherwise 404 if they are not a member or cannot see the resource (identical to "doesn't exist"), and 403 if they can see it but lack the scope for the action. **This is the most important security requirement of the tenancy design.** Community tests cover users without a membership and users with the wrong role, for liturgies, comments, PDF, share links and API endpoints.
5. **URL generation through `URLBuilder`.** All internal links, redirects, PDF links and share links are built by the `URLBuilder` port. The community implementation adds no prefix (`/liturgies/…`, `/api/v1/…`). No hardcoded paths in handlers or the frontend's link-building code.
6. **Share links.** Published-liturgy links (web view, PDF, "my assignments") are normal church URLs and require login; after login the user returns to the link they opened. Guest links without login are deferred (see decisions log).
7. **First-time setup** creates the install's one church and its first admin (see decisions log).

#### 8.1.1 SaaS multi-church hosting *(moves to `liturgist-saas`)*

The hosted SaaS serves every church from one product subdomain, with the church identified by the first path segment:

```
https://liturgist.brightfellow.net/<church_slug>/...
e.g. https://liturgist.brightfellow.net/gky-citragarden/liturgies/2026-10-11
```

- **Church slug.** Each church has a unique, lowercase, URL-safe slug (`a-z`, `0-9`, `-`; e.g. `gky-citragarden`). Validate on creation.
- **Reserved slugs.** Reject slugs that collide with system routes or could confuse users, at minimum: `login`, `logout`, `signup`, `api`, `admin`, `static`, `assets`, `health`, `docs`, `about`, `help`, `www`. Keep the list in one place and check it against the router's top-level routes in a test.
- **Slug resolver.** The SaaS `TenantResolver` reads the first path segment and loads the church; unknown slug → 404. The SaaS mounts the community API under `/{slug}` with this resolver in front, and its `URLBuilder` adds the `/<slug>` prefix.
- **Slug changes.** Prefer slugs to be immutable. If renaming is supported, store the old slug in `ChurchSlugRedirect` and 301-redirect old URLs, because links get printed and shared in WhatsApp groups.
- **Platform-level routes** (login, signup, platform admin) live outside any church prefix. After login, send the user to their church, or a church picker if they belong to several.
- **Cross-church tests.** All churches share one domain, so the session cookie is valid on every church's path. Dedicated tests: a user of church A requesting church B's liturgy, comments, PDF, share links and API endpoints through B's URLs.
- **Church onboarding** and **PostgreSQL row-level security policies** (see decisions log).


### 8.2 Extensibility and editions

**Business model guiding the design.** Every feature of the core liturgy workflow is open source and free: templates, liturgies, song library, readings, review workflow, PDF, sharing. The hosted SaaS earns money from hosting and convenience, plus features that carry real running or license costs (e.g. WhatsApp Business API notifications, licensed Bible text, managed backups, custom domains, email delivery, media storage, priority support). Do not gate core workflow features behind a paywall.

**Extension points, not dynamic plugins.** The core defines interfaces for the parts likely to vary. Implementations are compiled in; there is no dynamic plugin loading and no promised public plugin API in the MVP.

| Extension point | Responsibility | Community implementation (MVP) | Possible premium / later implementations |
|---|---|---|---|
| `Notifier` | Tell team members a liturgy was published or changed | Team summary, personal wa.me messages and change summary to copy or share (5.6); email when configured, later | WhatsApp Business API (automatic sending, reminders), managed email |
| `BibleTextProvider` | Return text for a reference + translation | Manual entry with reuse (5.4) | Licensed provider API, public-domain translations |
| `Exporter` | Render a liturgy to an output format | PDF, web view | Slides (PPTX/OpenLP/presenter), other print layouts |
| `Storage` | Store generated files and future media | Local filesystem | Object storage (S3-compatible) |
| `AuthProvider` | Authenticate users | Password login for invited accounts (see decisions log) | SSO / Google login, passkeys, email magic links |
| `EventBus` | Deliver live "liturgy changed" and presence events to open editors *(unused while live updates are on the icebox)* | In-memory (one process) | PostgreSQL `LISTEN/NOTIFY` across several server instances (SaaS) |
| `Importer` | Turn a file or pasted text into import candidates (songs, readings) | Paste and split, OpenLyrics, ChordPro; then EasyWorship 6/7 and `.pptx` for the pilot | `.docx`, other presentation software; AI-assisted extraction (opt-in) |

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
2. **Archiving frees a slot.** Members with `liturgy.manage` (by default Liturgist and Church admin) can archive a **Published** liturgy. Archiving is a manual action; the app never archives anything automatically. Archived liturgies are kept in full, are never deleted by the limit, and **remain viewable read-only on every plan**, including PDF export. Archiving manages slots and clutter; it does not take a church's history away.
3. **Deleting unpublished liturgies frees a slot.** Members with `liturgy.manage` (by default Liturgist and Church admin) can delete any liturgy that is not yet Published (Draft, In Review, Needs Revision, Approved), with a confirmation step. Deletion removes the liturgy with its items, assignments and comments. Published liturgies cannot be deleted, only archived. *(Amended 2026-10-05: neither can a liturgy that was published and then reopened; see the decisions log.)*
4. **Creation is blocked at the limit.** Creating a new liturgy (from a template, from scratch, by duplicating, or through "Prepare next week") when the church is at the limit is blocked with a message offering the ways forward: upgrade, archive a published liturgy, or delete an unpublished one. The message should list the oldest published liturgies with a one-click archive action, so a church can continue in seconds.
   - **"Prepare next week" at the limit:** the list shows how many of the week's liturgies fit within both limits. When the week needs more slots than are free, it offers one button: "Archive last week's N published liturgies and create these N" (only Published liturgies, only for members with `liturgy.manage`). The user can instead untick extras, or archive or delete other liturgies first. Archiving still happens only when the user presses the button; nothing is archived automatically.
5. **Restoring needs a slot.** Unarchiving a liturgy makes it count again, so it is only allowed when the church is under the limit.
6. **Existing work is never blocked.** Editing, reviewing, approving, publishing, viewing, and exporting liturgies that already exist are never affected by the limit; only creating new ones is.
7. **Downgrades are gentle.** If a church on a paid plan drops to free while over the limit, nothing is archived or locked automatically. The church simply cannot create new liturgies until it archives enough to get under the limit.
8. **Visibility.** Church settings and the liturgy list show usage (e.g. "6 of 7 active liturgies"), with a warning when one slot is left, so nobody discovers the limit on Saturday night.

**Rules for `max_unpublished_liturgies`:**

1. **What counts.** Every non-archived liturgy in an unpublished state: Draft, In Review, Needs Revision, or Approved. Counting all unpublished states (not only Draft) prevents working around the limit by moving liturgies into review.
2. **Both limits apply.** An unpublished liturgy counts toward both `max_unpublished_liturgies` and `max_active_liturgies`. Creating a liturgy requires a free slot in both.
3. **Freeing a slot.** Publishing a liturgy frees an unpublished slot (it still counts as active). Deleting an unpublished liturgy frees a slot in both limits.
4. **Blocked creation message** explains which limit was reached (also in "Prepare next week", per liturgy that doesn't fit). When it is the unpublished limit, list the unpublished liturgies with their state so the user can finish or delete one.
5. **Why 4:** a church with one weekly service can plan about a month ahead; a church with several services per Sunday can prepare the next Sunday fully plus some upcoming ones. It sits below the active limit of 7, leaving room for at least 3 published liturgies to stay active for reference.
6. Downgrades follow the same gentle rule as above: nothing is deleted or locked; creation is blocked until the church is under the limit.

**Rules for `max_team_members`:**

1. **What counts.** Every active membership in the church, whatever its roles (including members with no role), plus every pending, unexpired invite. Free-text names used in assignments are not counted and stay unlimited. A person who belongs to several churches counts once in each.
2. **Invites reserve a slot.** Creating an invite takes a slot, so accepting it never fails. Expired or cancelled invites free their slot.
3. Enforced at the moment of adding: inviting or adding a member beyond the limit is blocked with a clear upgrade message.
4. **Freeing a slot.** A church admin can remove a member or cancel an invite; the slot is freed immediately. "Remove member" is part of the MVP.
5. Existing members are never removed or locked out automatically, including after a downgrade; new invites are simply blocked until the count is under the limit.
6. Church settings show usage (e.g. "9 of 12 team members").

**Tests:** counting across all states and multiple services per date; blocked creation via every creation path (template, scratch, duplicate, "Prepare next week") for each limit separately; "Prepare next week" shows correctly how many liturgies fit and creates none beyond the limits; the combined archive-and-create button archives only Published liturgies, only for members with `liturgy.manage`, and only when pressed; archive only allowed for Published and only with `liturgy.manage`; delete only allowed for unpublished liturgies and only with `liturgy.manage`; archived liturgies viewable and exportable on the free plan; unarchive blocked at the limit; existing liturgies fully usable at and over the limit; downgrade behavior; blocked member invites; team-member count includes every role and pending invites but not free-text assignees; accepting a reserved invite never fails; expired, cancelled or removed entries free a slot.

### 8.3 Self-host operations

The person running a community install is usually a volunteer without IT training: every routine task should be one command or one button.

**Install and platforms**
- Supported release builds: Linux (amd64, arm64), Windows (amd64, able to install itself as a Windows service), and a Docker image (`ghcr.io/brightfellow-net/liturgist`). macOS is not a packaged or tested platform, but developers can build and run on it.
- Start with `liturgist serve` or `docker run -d -p 8080:8080 -v liturgist-data:/data ghcr.io/brightfellow-net/liturgist`. The binary listens only on `127.0.0.1:8080` unless configured otherwise (a TLS proxy or tunnel in front is the recommended setup); the Docker image listens on all interfaces inside the container. Nothing must be configured to start; options come from environment variables or an optional config file (base URL, database, email, HTTPS).
- One data folder holds everything: `liturgist.db`, `files/` (local `Storage`), `backups/`. Default `./data` (`/data` in Docker), or `LITURGIST_DATA_DIR`. An example systemd unit is provided.
- Runs comfortably on a 512 MB VPS, a Raspberry Pi 4, or an old Windows office PC.

**Upgrades**
- Replace the binary (or pull the new image) and restart. Migrations run automatically on startup, after writing a pre-upgrade copy of the database (`backups/pre-upgrade-<old version>.db` for SQLite).
- Downgrade protection: if the database is newer than the binary, the app refuses to start and says which version is needed. An `--allow-newer-schema` flag (off by default, for operators) overrides this when rolling back one version.
- Migrations are forward-only, and follow the expand/contract rule: a release never removes or renames something the previous release still uses, so the previous version can run on the current schema.
- `--no-auto-migrate` and `liturgist migrate` give manual control (also used by the SaaS, which migrates as a separate deploy step under a PostgreSQL lock).

**Backups and restore**
- `liturgist backup [file]` writes one archive: a consistent snapshot of the database, taken safely while the app runs, plus `files/`.
- Built-in automatic backups: daily at 02:00, keeping 7 daily and 4 weekly copies in `backups/`; configurable and can be turned off.
- A **"Download backup"** button on the church admin's system page, so a non-technical admin can keep a copy elsewhere (laptop, Google Drive).
- A "last backup" warning when no backup has been made or downloaded recently, because automatic backups on the same disk do not survive a disk failure.
- Off-site copies are documented (e.g. rclone, Litestream), not built in.
- `liturgist restore <file>`, with the server stopped: checks the file's integrity first and keeps the current database as `pre-restore-….db`.
- PostgreSQL self-hosters: documented `pg_dump` steps.

**HTTPS and access**
- Built-in automatic HTTPS (Let's Encrypt via `certmagic`) when a domain is set (e.g. `LITURGIST_DOMAIN`).
- Documented guides for a reverse proxy (Caddy, nginx) and for Cloudflare Tunnel or Tailscale (an office PC with no public IP address).
- Plain HTTP on the local network is allowed, with a warning on the system page that phones outside the network, secure cookies and the PWA won't work properly.

**Health, logs and disk space**
- `/healthz` (process alive) and `/readyz` (database reachable, migrations applied); the Docker image includes a health check.
- Logs via `slog` to standard output (text by default, JSON optional), with a request ID. **No personal data in logs** (no emails, phone numbers or lyrics).
- Disk space: a warning banner for church admins below a threshold (e.g. 1 GB or 5% free). Backups and pre-upgrade copies check for space first and skip with a clear message instead of filling the disk. A failed write shows users a clear "server storage is full" message.

**System page and releases**
- A system page for church admins: version, database size, free disk space, last backup, email configured or not, HTTPS status, update available (if enabled).
- Update check: **off by default**; a church admin can turn it on, after which the server asks GitHub for newer releases.
- Releases: semantic version tags (`v0.x` until stable), release notes, signed checksums, builds for every supported platform.

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

1. Project skeleton, data model, migrations (SQLite + PostgreSQL) with downgrade protection and the SQLite pre-upgrade copy, auth (with login throttling and trusted proxies), church + users + custom roles with the role editor, tenant context, `TenantResolver` and `URLBuilder` ports, membership tests (8.1), extension-point interfaces and entitlement stub with feature and limit checks (8.2, 8.2.1).
2. Song library (CRUD, sections, search) and readings store.
3. Templates and weekly liturgy editor (items, reorder, songs, readings, assignments).
4. Review workflow with item-level comments and state history.
5. Published web view, "my assignments" view, PDF output.
6. Self-host packaging per 8.3: release builds for Linux, Windows and Docker, `liturgist backup` / `restore` and scheduled backups, built-in HTTPS, health checks, system page, install and off-site backup guides, README.

Pilot with GKY Citragarden after step 5; gather feedback from the liturgist, administrator, and multimedia team before starting slides. The pilot plan (goal, success criteria, hosting, timeline, feedback, risk rules) is in [PILOT.md](PILOT.md).

## 11. Decisions log

Record resolved decisions here (date, decision, reason). Move items from section 9 here once settled.

| Date | Decision | Reason |
|---|---|---|
| 2026-10-02 | SaaS uses path-based tenancy: `liturgist.brightfellow.net/<church_slug>/` *(Amended 2026-10-02: multi-church hosting is SaaS-only; see the tenancy-split row below)* | One DNS record and one TLS certificate; simpler than per-church subdomains |
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
| 2026-10-02 | Clean architecture: domain → application (use cases + ports) → adapters, wired in one composition root by configuration. Persistence, search, transactions and the 8.2 extension points are ports; repositories are tenant-scoped | Core logic stays independent of the database and framework, so SQLite and PostgreSQL can be swapped without touching it; tenant-scoped ports enforce 8.1 rule 3 |
| 2026-10-02 | One shared SQL persistence adapter with a small dialect layer (placeholders, upserts, locking, full-text search), not separate SQLite and PostgreSQL adapters. Separate migration folders per database with matching versions | Each query is written once and the two databases can't silently drift apart; database-specific differences stay in one small place |
| 2026-10-02 | *(Amended 2026-10-02: a temporary SQLite file instead of in-memory, copied from a migrated template; see [02-persistence.md §6](impl/02-persistence.md#6-testing-strategy).)* Use-case tests run against the real SQLite adapter (in-memory), not hand-written fakes. Repository contract tests run against both SQLite and PostgreSQL (PostgreSQL in CI) | Fast enough for every test run; avoids keeping a third, fake persistence implementation; meets the "tests against both" requirement in 8 |
| 2026-10-02 | Tech stack: Go backend exposing a JSON API (the HTTP adapter over the use cases) + React SPA (Vite + TypeScript) for all web pages, built into the Go binary with `go:embed`. A React Native (Expo) mobile app may follow later, for team members and the multimedia team | Owner has 5 years of Go and knows React. One frontend approach on every page. The multimedia presenter needs client-side, offline-capable code, and React skills and TypeScript packages carry over to React Native. Self-hosters still get a single binary |
| 2026-10-02 | Backend libraries: chi router; Huma (code-first OpenAPI 3.1); `database/sql` + `sqlx` with hand-written SQL; `modernc.org/sqlite` (pure Go, no CGO) and `pgx`; goose migrations (embedded); ULID IDs; `log/slog`; `go-i18n`; testcontainers-go for PostgreSQL in CI; `depguard` to enforce layer boundaries | Small, stable dependencies; static single binary; OpenAPI generated from Go types keeps the TypeScript client in sync |
| 2026-10-02 | Frontend libraries: `openapi-typescript` + `openapi-fetch` (types and client generated from the OpenAPI spec); TanStack Query; React Router; React Hook Form + Zod; Tailwind + shadcn/ui; dnd-kit; i18next (same message files as Go, `id` default, `en` second *(amended 2026-10-02: English first; see the UI-language row below)*); `vite-plugin-pwa`; Vitest + Testing Library; Playwright for end-to-end | Common, maintained choices. API client, types and i18n can be shared with a future React Native app. shadcn/ui code lives in the repo, which limits dependency churn |
| 2026-10-02 | Sessions are server-side and stored in the database, in an HttpOnly SameSite=Lax cookie with CSRF protection; a mobile app later uses bearer tokens through `AuthProvider`. *(The login method was resolved later the same day; see the auth row below.)* | Sessions can be revoked and work the same on SQLite and PostgreSQL |
| 2026-10-02 | *(Superseded 2026-10-02 by the tenancy-split row below.)* URL layout: in `multi` mode the API is at `/<church_slug>/api/v1/...` and SPA routes at `/<church_slug>/...` (the server returns `index.html`); in `single` mode `/api/v1/...` and `/...`. Platform routes (login, platform API) sit outside any church prefix | Keeps 8.1 unchanged: the first path segment is the church, so the tenant middleware and URL helper work for both API and pages |
| 2026-10-02 | The API returns the actions the current user may take on each resource (e.g. `"actions": {"submit": true, "edit": false}`). The UI shows or hides controls from these and never re-implements permission or workflow rules | All rules stay in Go, in one place; web and mobile clients stay consistent |
| 2026-10-02 | *(Amended 2026-10-02: undo is per person, not shared; see the concurrent-editing row below.)* Undo/redo in the liturgy editor is server-side: each editor change is a use-case command recorded per liturgy with the data to reverse it. History is shared by everyone editing that liturgy and lasts after reload, while the liturgy is editable (Draft, Needs Revision). The UI may apply undo immediately and confirm with the server in the background | History is the same on every device and for both editors; it can also show what changed since the last review |
| 2026-10-02 | PDF: print stylesheet on the published view for the MVP; server-side Typst later, shipped alongside the binary (in the Docker image or release archive). PDF does not need to be inside the single binary | Server cost is negligible, and server output is the same on every device and shareable by URL; the final layout waits for open decision 3 |
| 2026-10-02 | Repository layout: one monorepo with the Go module at the root (`domain/`, `app/`, `adapters/…`, `server/`, `cmd/liturgist/`, `internal/` for private helpers only, `migrations/{sqlite,postgres}`; see the package-layout row below, which replaced the earlier `internal/{domain,app,adapters}`) and pnpm workspaces for TypeScript (`web/`, `packages/api-client`, `packages/i18n`). `make build` builds the frontend, generates API types, then runs `go build`; CI fails if generated types are out of date | One place for the whole web product. How a future mobile app shares the API client and translations is decided later, with the mobile app |
| 2026-10-02 | Auth for MVP: invite-only accounts with a password. A church admin creates an invite (link expires in 7 days) and shares it through any channel; the recipient sets a password. No open sign-up into a church. Passwords: argon2id, minimum 10 characters, checked against a list of commonly leaked passwords, no composition rules, rate-limited login attempts. Google login, passkeys, email magic links and two-step login for admins come later through `AuthProvider` | Works without email setup, which matters for self-hosters; invite links can be shared on WhatsApp; churches control who joins |
| 2026-10-02 | Login identifier is email or phone number; each user has at least one and may have both, and each is unique across the platform. Emails are stored trimmed and lowercased; phone numbers in international format, with Indonesia as the default region (`0812…` → `+62812…`). One "Email or phone number" field on the login form | Many volunteers use WhatsApp more than email; the identifier needs no verification because an admin invited the person |
| 2026-10-02 | Password reset: by email link when the user has an email and the install has email configured; otherwise a church admin generates a reset link to share; a `liturgist user reset-password` command for locked-out admins | Every install can recover accounts, with or without email |
| 2026-10-02 | Every team member has a full account, including read-only team members | Owner's choice: one consistent way to access the app |
| 2026-10-02 | Users are platform-wide (`User` has no `church_id`); church access and roles live in `Membership`. Inviting an email or phone that already belongs to a user adds a membership after they log in, and never creates a duplicate account | Supports users in several churches on the SaaS (8.1.1); in a community install everyone simply has one membership |
| 2026-10-02 | *(Amended 2026-10-02: in the community edition setup creates the install's only church. See the tenancy-split row below.)* First-time setup: a one-time setup link printed to the log on first start opens a "create church and first admin" page; a `liturgist setup` command does the same for scripts and Docker. Both call the same use case | Friendly for non-technical installers, and scriptable for automated installs |
| 2026-10-02 | Share links require login (8.1 rule 6): the published view, its PDF and "my assignments" are only shown to logged-in members of that church, and after login the user returns to the link they opened. Guest links are deferred; if the pilot asks for them, they will be per-liturgy tokens (stored hashed, read-only, expiring a week after the service by default, revocable) | Lyrics and Bible text are copyrighted, and church licences usually cover the congregation, not anyone holding a link; one access rule keeps the 8.1 rule 4 tests sufficient |
| 2026-10-02 | Every member of a church can view all of that church's published liturgies, not only those they are assigned to | Simpler, and lets musicians and readers look ahead |
| 2026-10-02 | Publishing stores a `PublishedVersion`: a complete copy of the liturgy as published. The published view and PDF always read from the latest version, never from the editable liturgy. When a published liturgy is reopened, members keep seeing the last published version with a "being revised" notice; edits stay invisible until the next publish. Archiving keeps all versions | The team always has something to rehearse from; later library edits don't change what was distributed; exact record for future licence reporting (5.3) |
| 2026-10-02 | Sessions last 90 days, extended whenever the app is used; configurable per install | Team members rarely need to log in again on their phones |
| 2026-10-02 | SaaS database layout (open decision 6): one shared PostgreSQL database with `church_id` on every church-owned row. Indexes on church-owned tables start with `church_id`; foreign keys between church-owned tables include `church_id`; deleting a church cascades from `Church`. Not one SQLite file per church, and not one schema per church | Users and memberships are platform-wide, so per-church files would still need a separate platform database; one migration run, managed backups and simple cross-church reporting; expected scale is easily handled by one PostgreSQL instance |
| 2026-10-02 | *(Amended 2026-10-02: the policies are SaaS migrations; the hook stays in the shared adapter. See the tenancy-split row below.)* PostgreSQL row-level security as a second line of defence: the hook is built now (the PostgreSQL dialect sets the current church at the start of each transaction through the transaction port); policies on church-owned tables are switched on before the SaaS launch. Platform-level queries (login, membership lookup) use a separate path allowed to bypass them. SQLite has no equivalent and relies on tenant-scoped repositories | Catches a buggy query in our own code before it can return another church's data; building the hook now is cheap and avoids retrofitting |
| 2026-10-02 | *(Amended 2026-10-02: in the community edition the command exports the install's only church, `liturgist church export`; exporting by slug is a SaaS command. See the tenancy-split row below.)* Export one church to a SQLite file and import it again (`liturgist church export <slug>` / import), built after the MVP and before the SaaS launch, reusing the shared SQL adapter | Gives back the main advantage of per-church files: a church can take its data to self-hosting, or be restored on its own |
| 2026-10-02 | `max_team_members` (open decision 8) counts every active membership in the church, whatever its roles, plus pending unexpired invites, which reserve a slot so accepting never fails. Free-text assignees are not counted. A person in several churches counts once in each. A church admin frees a slot by removing a member or cancelling an invite; "remove member" is in the MVP | Clear and easy to explain ("12 people can use the app"). Free-text names are for guests and would otherwise block edits to existing liturgies; people without accounts can't log in, so using them as a workaround gains little |
| 2026-10-02 | Main translation for the pilot is LAI Terjemahan Baru (TB), copyrighted by LAI. The owner will contact LAI about licensing (church-use rules, a licence for the hosted service, storing text, required attribution, TB2, cost) | TB is what GKY Citragarden uses; a licence from LAI (directly or through an API that carries TB) is the only clean way to provide TB text in the app |
| 2026-10-02 | Bible text in the MVP: manual entry with reuse (5.4), plus a reference parser that understands Indonesian book names and abbreviations (e.g. "Yoh 3:16-21", "Kej. 1:1–2:3", "Mzm 23") and stores references in a standard form (standard book codes, e.g. `JHN 3:16-21`). Each church has a default translation. Stored readings carry an attribution line, shown on the published view and PDF. Different verse numbering between translations is allowed for in the design but not handled in the MVP | Manual entry needs no licence from the app; standard references let any later provider look text up; most licences require attribution |
| 2026-10-02 | Bible text after the MVP: an import provider for Bible files the church has rights to (USFM, OSIS or Zefania XML), and an optional download of public-domain and open-licence texts (e.g. Chinese Union Version 1919, KJV, WEB, BSB; AYT if its licence is confirmed). Nothing is bundled in the binary, so "ships no Bible text" stays true. Licensed TB/TB2 for the SaaS depends on LAI's answer. Copying text from websites is not allowed | Fills whole Bibles in one step for self-hosters and gives Mandarin and English quickly, without distributing copyrighted text |
| 2026-10-02 | Each `BibleTextProvider` result states its source, its attribution line, and whether the text may be stored; stored copies (in `Reading` and `PublishedVersion`) record which provider they came from | API and publisher licences often limit storing text and require attribution; recording the source keeps each stored copy traceable to its licence |
| 2026-10-02 | Repository (open decision 5): `brightfellow-net/liturgist` (Go module `github.com/brightfellow-net/liturgist`, binary `liturgist`, Docker image `ghcr.io/brightfellow-net/liturgist`). The private SaaS build lives in `brightfellow-net/liturgist-saas` (premium implementations, the SaaS `main`, deployment config). A future mobile app lives in a separate repository; its name and how it shares code are decided later | Matches the product name, the SaaS URL and the CLI commands in this spec; the org name already says Brightfellow, so no prefix is needed |
| 2026-10-02 | This repository is the community edition only. `liturgist-saas` imports its Go module. SaaS-specific content in this spec (free-plan limit values, plan reasoning, SaaS onboarding) moves to `liturgist-saas` later; the hooks stay here (entitlement stub and limit-check calls, the tenancy foundation and its ports, the row-level security hook) *(Amended 2026-10-02: `multi` mode moved to the SaaS; see the tenancy-split row below)* | Keeps the open repository complete and independent |
| 2026-10-02 | Go package layout: packages the SaaS imports live outside `internal/`, at the top level: `domain/` (entities, state machine, rules), `app/` (use cases and ports), `adapters/…` (e.g. `sqlstore`, `httpapi`), and `server/`, a builder that wires everything and accepts options (e.g. `WithNotifier`, `WithEntitlements`, `WithRoutes`). `cmd/liturgist/` is the community `main`. `internal/` is only for private helpers | Go does not allow other modules to import `internal/` packages; one builder with options keeps the SaaS `main` small |
| 2026-10-02 | The SaaS customises the API only by adding endpoints, registered as extra Huma operations through a `server` option; it never overrides or changes community endpoints. Differences in behaviour go through ports (`Entitlements`, `Notifier`, `BibleTextProvider`, …). The SaaS pins `v0.x` tags of this module, a `go.work` file links both repos during development, and SaaS CI runs nightly against community `main` | Avoids forked endpoints drifting from the community version; breaking changes are caught early |
| 2026-10-02 | The SaaS frontend lives in a separate repository as an independent React app, built against the SaaS OpenAPI spec (community operations plus SaaS operations). The community frontend is not built as a reusable package. The API must stay independent of any one frontend (no endpoints shaped around a particular screen), and all rules stay on the server ("allowed actions"), so both frontends behave the same | Owner's choice: full freedom for the SaaS presentation layer. The cost is building core screens twice, and the API becomes the shared contract |
| 2026-10-02 | Tenancy split: each community install serves exactly one church; to serve several churches, run several instances. The community edition keeps the foundation: `church_id` on church-owned rows, a tenant context on every request, church-scoped repositories, memberships, permission checks in the use cases, and two ports, `TenantResolver` (community: the single church from config) and `URLBuilder` (community: no prefix; API at `/api/v1/...`). `single`/`multi` stop being configuration options. The SaaS owns the multi-church mechanics: slug routing and rules, reserved slugs, slug redirects, the church picker, platform routes and admin, church onboarding, cross-church URL tests, and the row-level security policies. It mounts the community API under `/{slug}` with its own resolver | Keeps the community code simpler and the paid product's value clear, without forking: the SaaS builds on the same schema and use cases. Accepted cost: a synod or volunteer hosting many branches needs several instances or the SaaS |
| 2026-10-02 | Song sequences: a liturgy item can hold several songs (medley, `LiturgyItemSong`). Each song has an ordered sequence (`SequenceEntry`) of sections with repeats allowed, each entry optionally naming who sings it (`SingingPart`, configurable per church, seeded with Semua, Pemandu, Jemaat, Pria, Wanita, Paduan Suara) and carrying a key change and a note. Songs have an optional default arrangement that fills a new sequence (otherwise all verses in order). Key per song in the liturgy, defaulting to the song's key, displayed as "Do = G" by default (church setting). `SongSection` gains kind and number. Non-lyric entries (instrumental, spoken) are not in the MVP; `SequenceEntry.kind` leaves room for them. A section cannot be deleted while an unpublished liturgy uses it. Replaces `LiturgyItem.song_id` and `song_section_order` | Matches how Indonesian liturgies are sung and printed (alternating parts, repeats, modulation, medleys, "Do = G"); default arrangements save rebuilding the same order every week; sequence entries map directly to slide groups later |
| 2026-10-02 | Concurrent editing: version check on each change plus live updates, not locks or real-time co-editing. `Liturgy` and `LiturgyItem` carry a version; each change states the version it was based on, and the server rejects only real conflicts (the same item changed meanwhile; the liturgy version for reordering and state changes). Conflicts show "changed meanwhile" and keep the user's text. Changes and presence ("Budi is also editing", in the MVP) are pushed over server-sent events through a new `EventBus` port: in-memory in the community edition, PostgreSQL `LISTEN/NOTIFY` in the SaaS | Overlap is uncommon but must never lose edits silently; the editor already sends small named changes, so only real conflicts need rejecting; no stale locks |
| 2026-10-02 | Undo is per person: Ctrl+Z undoes your own latest change and is refused if someone has since changed that item. The history (`LiturgyEdit`) stays per liturgy and visible to everyone | With two editors, a shared Ctrl+Z would undo the other person's change; per-person undo is what Google Docs and Figma users expect |
| 2026-10-02 | In Review is read-only apart from comments; only Draft and Needs Revision are editable. To change a liturgy under review, the reviewer requests changes | Clear handoff between author and reviewer, and far fewer simultaneous edits |
| 2026-10-02 | Content language: BCP 47 language codes on `Liturgy` (the service's main language), `Template`, `Song` and `Church` (`default_language`). Readings refer to a `Translation` record (code, name, language, e.g. `TB` → `id`, `CUV` → `zh-Hans`). The same song in another language is a separate `Song`, linked through a `SongGroup`. Content stays in one language per record for now; parallel text later adds secondary-language text alongside it. Bilingual singing meanwhile uses a medley of the linked versions. Mandarin uses simplified characters (`zh-Hans`). Pinyin comes later | The pilot church needs more than one language. Language versions of a hymn differ in hymnal number, copyright and verse count, so separate linked songs fit better than per-section translations; the model leaves room for parallel text without a rewrite |
| 2026-10-02 | Lyric search uses substring matching for Chinese text and full-text search for Indonesian and English, both behind the search interface | Chinese has no spaces between words, so full-text search (and 3-character trigram search) misses typical 1–2-character queries; libraries are small enough for substring matching |
| 2026-10-02 | Self-host platforms: release builds for Linux (amd64, arm64) and Windows (amd64, can run as a Windows service), plus the Docker image. macOS is not a packaged or tested platform | Many Indonesian churches have a Windows office PC; pure Go makes cross-platform builds cheap |
| 2026-10-02 | Upgrades: migrations run automatically on startup after writing a pre-upgrade copy of the database; the app refuses to start on a database newer than the binary; `--no-auto-migrate` and `liturgist migrate` give manual control | Upgrading becomes "replace and restart", with a way back if something goes wrong |
| 2026-10-02 | Backups: `liturgist backup` writes one archive (database snapshot plus `files/`); built-in daily backups keep 7 daily and 4 weekly copies; a "Download backup" button on the system page; a "last backup" warning; `liturgist restore` checks integrity and keeps a pre-restore copy. Off-site copies are documented, not built in | Protects against mistakes automatically, and lets a non-technical admin keep a copy off the server; managed off-site backups remain a SaaS service |
| 2026-10-02 | HTTPS: built-in automatic HTTPS (Let's Encrypt via `certmagic`) when a domain is set; documented guides for a reverse proxy and for Cloudflare Tunnel/Tailscale; plain HTTP on the local network allowed with a warning | Phones outside church need HTTPS for secure cookies and the PWA; office PCs often have no public IP address |
| 2026-10-02 | Update check is off by default; a church admin can turn it on | The server should not contact outside services without the church's consent |
| 2026-10-02 | Operations basics: `/healthz` and `/readyz`; `slog` logs with request IDs and no personal data; disk-space warnings, with backups checking for space first; a system page for church admins; releases with semantic versions, release notes and signed checksums | Gives volunteers visibility without command-line skills; keeps logs safe under UU PDP |
| 2026-10-02 | Importing content: an `Importer` port; paste and split (with Indonesian, English and Chinese section labels), OpenLyrics and ChordPro in the MVP; EasyWorship 6/7 (version to be confirmed with the pilot church; OpenLP's importer plus OpenLyrics as the fallback route) and `.pptx` for the pilot; `.docx`, spreadsheets and other formats later. File imports go through a review step (`ImportBatch`, `ImportCandidate`) with duplicate detection by hymnal number or normalised title; accepted candidates are saved through the normal use cases | An empty library is the biggest hurdle to adoption; the pilot church is assumed to use EasyWorship and PowerPoint, and EasyWorship already holds lyrics split into sections |
| 2026-10-02 | AI-assisted import is a later, opt-in option only (bring-your-own key, or a SaaS service with the church's explicit consent). Past liturgies are not imported as history | AI extraction sends church content to an outside service and must not be required for self-hosting; old services add little value compared with songs |
| 2026-10-02 | First-time experience: a setup wizard (church and first admin, default language, default translation, time zone, key display, regular services); seeded editable defaults in the church's language (role types, singing parts, translation names without text, a starter template with titles only, to be aligned with GKY's order of service); an onboarding checklist and helpful empty screens; a team-member welcome on "Tugas saya" with home-screen instructions. No sample data in installs; a public demo site later | A church should reach its first real liturgy quickly without help; Bible text and hymn lyrics can't be shipped as defaults or samples |
| 2026-10-02 | Services: a church defines regular services (`Service`: name, language, default template) with one or more weekly times on any weekday (`ServiceTime`). "Prepare next week" lists all scheduled occurrences and creates the ticked ones in one click. One-off services are created individually. Liturgies keep a copy of the service name. Monthly patterns are later | Most services repeat weekly, but not only on Sundays and sometimes on several days; preparing a whole week at once saves the most time |
| 2026-10-02 | "Prepare next week" respects the free-plan limits: it shows how many liturgies fit, and when more slots are needed it offers one explicit button, "Archive last week's N published liturgies and create these N". Automatic archiving was considered and rejected | Nearly as convenient as auto-archiving while keeping the earlier decision that nothing is archived automatically and the church stays in control; only relevant to the SaaS free plan (community installs are unlimited) |
| 2026-10-02 | WhatsApp messages in the MVP (community `Notifier`): a team summary (Copy, Share to WhatsApp via `wa.me/?text=`), personal messages per assigned person (Send via WhatsApp via `wa.me/<phone>?text=`), and a change summary after republishing (diff against the previous `PublishedVersion`). Content: titles, hymnal numbers, keys, reading references, assignments and the link; never lyrics or Bible text; checkboxes for songs, keys and readings. WhatsApp `*bold*` and plain URLs only. Friendly tone with the person's name, avoiding "kamu" or "Bapak/Ibu". Editable message templates, automatic sending, reminders and email notifications come later | WhatsApp is how Indonesian church teams communicate; wa.me links give most of the value with no API, cost or setup; copyrighted text stays behind login |
| 2026-10-02 | Print formats in the MVP: team version and musician sheet; A4 and F4 / Folio paper; options for lyrics (full or first lines), reading text, assignments, keys and notes, and text size, with church defaults; header with church name, optional logo (stored through `Storage`), service, date and time. Produced with print styling in the browser. Congregation version, "my part" and A5 booklets come later (booklets need server-side Typst plus a booklet step). The exact layout still waits for open decision 3 | F4 is common in Indonesian churches and easy to forget; the musician sheet costs almost nothing from the same data; booklets can't be produced reliably by browsers |
| 2026-10-02 | Song copyright: `Song.author` is split into lyricist, composer and translator, and songs gain copyright holder, copyright line, CCLI song number and licence status (unknown, public domain, covered by church licence, permission obtained), keeping free-text notes; the library can filter by status. Church licences (e.g. CCLI number) live in settings, with an optional licence footer on prints. Credit lines appear under songs in the team version and published view, on by default. `PublishedVersion` stores the credits as published. A usage report CSV is in the MVP. Licence status never blocks publishing or printing | Licences usually require credit lines and usage reporting; translations of old hymns are separate copyrighted works; the app records and reports but doesn't police, because it can't know a church's agreements |
| 2026-10-02 | Older volunteers and accessibility: the UI follows the phone's text size (works at 200%), plus an A · A+ · A++ control saved to the account (`User.preferences`). Reading mode in the MVP: large text in one column, high contrast, the viewer's own items highlighted with "Go to my part", a keep-screen-on toggle (Wake Lock), offline through the PWA. People who are only team members see just "Tugas saya" and "Liturgi". WCAG 2.2 AA as the target with automated axe checks; tap targets of at least 48 px, labelled buttons, plain Indonesian, dark mode following the phone | Many team members are older and read from phones at the lectern; these are cheap to build in from the start and expensive to retrofit |
| 2026-10-02 | Pilot plan recorded in `docs/PILOT.md`: owner-hosted community edition during the pilot (then self-hosting or SaaS); one Indonesian service first, about 8 weeks with a 2-week parallel run; baseline and success criteria; feedback through a WhatsApp group, an in-app feedback link, weekly check-ins, end interviews, a survey and a test with older volunteers; no updates Friday to Sunday, a weekend contact, tested backups | A plan is project work rather than product specification, so it lives in its own file; the product requirements it needs are in this spec |
| 2026-10-02 | Product requirements from the pilot plan, for every install: no analytics or tracking scripts; a privacy notice page (linked from login and invite pages); an optional "Kirim masukan" feedback link set in settings; `User.last_seen_at` for usage figures from the app's own database | Personal data of church members is covered by UU PDP; usage can be measured without tracking; feedback should be one tap away |
| 2026-10-02 | UI language: English first, Indonesian second. English is the source language for UI text (`en.json` is the reference; `id.json` must have every key). Each church has a default UI language for its members (`Church.default_ui_language`, chosen in the setup wizard, English pre-selected); each user can override it in `User.preferences`. Content language is unaffected. WhatsApp messages use the liturgy's language when a translation exists, otherwise the church's default UI language. The accessibility rule becomes "plain wording in every language" | Owner's choice. A church-level default lets an Indonesian church give its volunteers Indonesian without each person switching; messages follow the service so an Indonesian team gets Indonesian messages |
| 2026-10-02 | Roles are defined by each church from fixed scopes (`church.settings`, `members.view`, `members.manage`, `roles.manage`, `library.edit`, `templates.edit`, `liturgy.edit`, `liturgy.comment`, `liturgy.approve`, `liturgy.manage`). Every church starts with three editable ready-made roles: Church admin, Liturgist, Editor. A member with no role is a team member (baseline: view published liturgies, my assignments, own profile). Safeguards: at least one member always holds `roles.manage` and `members.manage`; no granting of scopes one doesn't hold; code checks scopes, never role names. The role editor is part of build step 1. Liturgy tasks (formerly `RoleType`) are renamed **duties** and grant no permissions | "Church admin" and "Administrator" were easy to confuse; letting each church name its roles avoids that and fits churches organised differently from GKY. Ready-made roles keep the default simple. `liturgy.manage` keeps the earlier decision that both liturgist and church admin can archive and delete |
| 2026-10-02 | Migrations: forward-only (no down migrations); downgrade protection with an `--allow-newer-schema` override for operators (off by default); the expand/contract rule for removing or renaming schema parts; the SQLite pre-upgrade copy moves from build step 6 to step 1 | Down migrations can't restore dropped data and rarely get tested; the pilot starts after step 5 and needs the pre-upgrade copy before then; expand/contract allows rolling back the program one version and SaaS rolling deploys |
| 2026-10-02 | Sessions also have an absolute maximum lifetime of 1 year (configurable), even when used daily. Every login creates a new session token and deletes a session the browser already had. A "Log out on all other devices" button on the profile page is in build step 1; a devices list and an admin "log member out everywhere" action are later | Limits how long a stolen cookie works; prevents session fixation; gives volunteers a simple answer to a lost phone |
| 2026-10-02 | Setup link: the token is stored as a hash in the database and valid 24 hours; each start while not set up, and a `liturgist setup-link` command, print a fresh link in a framed block. Releases that add seeded defaults also seed them into existing churches | Volunteers running Docker from a dashboard can get a link with one command instead of searching logs; churches set up early still receive later defaults |
| 2026-10-02 | The community install finds its church in the database (the only `churches` row, cached), not in configuration; it refuses to start if the database holds more than one church | The church is created at runtime by setup, and "nothing must be configured to start"; never silently picking one church protects against restored or imported databases |
| 2026-10-02 | Response codes for access: 401 when not logged in; 404 when not a member or when the resource is not visible to the member, with bodies identical to "doesn't exist" and the reason only in the log; 403 when the member can see the resource but lacks the scope | Reveals nothing to outsiders (important for the SaaS) while giving members a clear "no permission" message |
| 2026-10-02 | All extension-point interfaces are defined in build step 1 and marked provisional until the step that first uses them. Until v1.0 any port may change; port changes are listed under "Ports" in `CHANGELOG.md` | Owner's choice: the full set of extension points is visible from the start; the changelog keeps SaaS updates manageable |
| 2026-10-02 | Step-1 implementation documents (`docs/impl/`, `docs/reference/schema.md`) approved: proposals P-01 to P-32 | Recorded in [docs/impl/README.md](impl/README.md#4-proposed-decisions) |
| 2026-10-02 | Adversarial review round 1 (index only) handled: P-33 (all unsafe API requests must be JSON), P-34 (per-church lock before role safeguards), P-35 (throttle-clearing command and proxy misconfiguration warning), P-36 (precedence between SPEC and implementation documents), plus clarifications | Recorded in the [review log](impl/README.md#6-review-log) |
| 2026-10-02 | Adversarial review round 2 (implementation documents and schema) handled: atomic operations for all single-use tokens, counters and setup (P-37); binary listens on 127.0.0.1 by default (P-38); allowed-host check and supported deployment setups (P-39); live invite roles in a join table (P-40); optional strict upgrade mode (P-41); setup link in logs accepted as a risk (P-42); changing one's own email/phone deferred (P-43) | All CRITICAL findings fixed and every HIGH finding decided; see the [review log](impl/README.md#6-review-log) |
| 2026-10-02 | Login throttling uses three counters (identifier + IP: 5 failures/15 min; identifier: 50/hour; IP: 100/15 min) and trusts client IPs only from configured reverse proxies, both in build step 1 | Most installs sit behind a proxy or tunnel; Indonesian mobile carriers and church Wi-Fi share IP addresses; counting per identifier + IP stops one person locking another out |
| 2026-10-02 | The ready-made Church admin role holds all ten scopes (amends P-15) | With the earlier six scopes, safeguard 2 meant nobody in a new church could ever give the Liturgist or Editor role or any `library.*`/`liturgy.*` scope; found while testing the members page in build step 1 |
| 2026-10-03 | The setup wizard does not ask for regular services; a church adds them on the Services page, which explains what a service is when empty (amends §5.7) | The wizard of step 1 is approved and merged, and services are a two-minute task with a helpful empty state; a wizard step can be added with the onboarding checklist later |
| 2026-10-03 | The ready-made Liturgist role also holds `templates.edit` (amends the scope table in 4) | The liturgist plans the weekly structure, so they need to maintain templates, services, duties and singing parts without asking a Church admin. Churches that already exist keep the scopes of their current roles; only roles created from now on get it |
| 2026-10-03 | Undo in the liturgy editor: a refused undo of a non-structural edit is skipped afterwards, and a state change of the review workflow starts a new undo window (amends the 2026-10-02 per-person undo rows) | A review of the rules on random two-user histories showed a refused edit would block every older one, and an undo reaching back past a review round could undo something the reviewer had read. Structural edits are never skipped, because restoring a removed item by position needs the later ones reversed first |
| 2026-10-03 | Live updates and presence ("Budi is also editing") go on the icebox and are not built in step 3; slice 3D is undo and redo only. Concurrent editing stays protected by the version checks and the conflict screen. A throwaway spike had shown that an event stream with a 20-second keepalive survives nginx and Caddy. This amends the 2026-10-02 concurrent-editing row (live updates over `EventBus`) | Saves development time; the versions already prevent lost edits. The design stays in [11 §7.1](impl/11-liturgy-editor.md#71-live-updates-and-presence-icebox-p-66) |
| 2026-10-04 | Review workflow (step 4): the build goes up to Approved; publishing and `PublishedVersion` move to step 5. Comments are flat (an item or the whole liturgy, a resolve flag, no replies, edit or delete). The submitter may approve; open comments only warn. A submit is refused for a liturgy with no items or with unfinished items. Approve and request changes must state the `edit_seq` the reviewer saw. Review comments and state history are visible only with a `liturgy.*` scope, even on a published liturgy | Keeps the freeze rules in one step with the snapshot; a small church may have one person holding both scopes; a stale browser tab must not approve unread content; the team sees the published result, not the reviewers' discussion |
| 2026-10-05 | Publishing (step 5): `publish` is a transition that makes an immutable `PublishedVersion` in the same transaction; a liturgy that has any version can never be deleted, even after a reopen (it is freed by republishing and archiving); members without a `liturgy.*` scope read published liturgies only through the published routes, never the editable view or its history; reopening a published liturgy is allowed even when references were cleared (shown as problems) and is subject to `max_unpublished_liturgies`; print by stylesheet only with a generic layout until the GKY document arrives; the church logo, older-version pages and server-side PDF are left for later; phone numbers for WhatsApp chats are shown only to members who can approve | Keeps the distributed copy and the licence record safe from races and from a reopen-then-delete; the editable liturgy and its edit history are not for the whole congregation; the owner chose the scope on 2026-10-05 and approved P-75 to P-84 ([impl/13-publishing.md](impl/13-publishing.md)) |

## 12. References

| Topic | Document | Type |
|---|---|---|
| Index of implementation documents and proposed decisions | [docs/impl/README.md](impl/README.md) | Implementation |
| Repository layout, build, configuration, server, logging, errors | [docs/impl/01-foundation.md](impl/01-foundation.md) | Implementation |
| Persistence: dialects, transactions, migrations, contract tests | [docs/impl/02-persistence.md](impl/02-persistence.md) | Implementation |
| Identity and auth: users, memberships, roles, sessions, invites, resets, setup | [docs/impl/03-identity-auth.md](impl/03-identity-auth.md) | Implementation |
| Tenancy, authorization, extension points, entitlements | [docs/impl/04-tenancy-extensions.md](impl/04-tenancy-extensions.md) | Implementation |
| Publishing (step 5) | [docs/impl/13-publishing.md](impl/13-publishing.md) | Implementation |
| Web app shell for step 1 | [docs/impl/05-web-shell.md](impl/05-web-shell.md) | Implementation |
| Database schema (tables, columns, constraints) | [docs/reference/schema.md](reference/schema.md) | Reference |
| Pilot plan | [docs/PILOT.md](PILOT.md) | Project plan |
