# Changelog

Before v1.0 any port (every interface in `app`, and `httpapi.TenantResolver`) may change. Every change to a port's signature or meaning is listed under **Ports** for its release ([04 §7](docs/impl/04-tenancy-extensions.md#7-extension-points)).

## Unreleased

Nothing yet.

## 0.1.0

The first release, made for the pilot at GKY Citragarden. It is for one church that runs it on its own computer; there is no hosted version yet.

### What is in it

- **Planning:** a setup wizard, members with roles you define, a song library (paste, ChordPro and OpenLyrics import, sections, copyright details), Bible readings by reference, duties, singing parts, templates and regular services, and "Prepare next week".
- **The weekly liturgy:** an editor for items, songs, readings and assignments, with history and undo and redo.
- **Review and publishing:** review states with notes and comments on the liturgy and on single items, publishing as numbered versions, archiving, and a published view with print, "Tugas saya" (my assignments), an offline reading mode, and WhatsApp messages with a summary of what changed.
- **Self-hosting:** one program or one Docker image (31.7 MB, linux/amd64 and arm64) with SQLite by default and PostgreSQL as an option; `liturgist backup` and `restore`, nightly backups with a retention rule, a System page for the church admin, a health check, a Linux systemd unit, a Windows service, and automatic HTTPS from Let's Encrypt when you set `LITURGIST_DOMAIN` and `LITURGIST_ACME_AGREE=true`. Guides for each are in [docs/self-host](docs/self-host/).
- **Languages:** the interface is in English and Indonesian (English first). Content can be in Indonesian, English and Chinese.

### Known limits

- The default duties, singing parts and starter template are a draft; the order of service and the English and Chinese words are not yet checked against GKY Citragarden's own ([Q-3.3](docs/impl/README.md#4c-questions-for-the-owner-step-3)). Churches can edit all of them.
- The Windows service and its guide have not been tried on a real Windows machine; the Windows log file is not trimmed.
- Bible text and hymn lyrics are not shipped: you add them yourself. There is no e-mail, no push notification and no slides or PDF export yet (print uses the browser's "Save as PDF").
- The Indonesian texts of recent screens are waiting for an owner review.
- Built-in HTTPS was tested against a local test CA, not yet against the real Let's Encrypt.

### Ports

- First definitions, all provisional [P-27]:
  - persistence: `Tx`, `Store`, `ChurchStore` and their repositories, `Clock`, `IDGenerator` ([02 §2](docs/impl/02-persistence.md#2-ports-in-app));
  - identity: `PasswordHasher` ([03 §3](docs/impl/03-identity-auth.md#3-passwords));
  - tenancy: `httpapi.TenantResolver`, `URLBuilder` ([04 §3–§4](docs/impl/04-tenancy-extensions.md#3-tenantresolver));
  - extension points: `Entitlements`, `AuthProvider`, `Storage`, `EventBus`, `Notifier`, `BibleTextProvider`, `Exporter`, `Importer` ([04 §7](docs/impl/04-tenancy-extensions.md#7-extension-points)).
- Placeholder types without fields, designed in the step that first uses them (`domain.Reference`, `app.BibleText`, `app.ImportHint` and `app.ImportCandidate` since step 2): `app.NotifyEvent`, `app.Message`, `app.PublishedVersion`.
- `AuthProvider` is defined but not used yet: password login stays in `app.Auth` until a second provider arrives.
- Step 2:
  - song library: `ChurchStore.Songs()` with `SongRepo`, and `SongUsage` ([06 §6](docs/impl/06-song-library.md#6-ports));
  - readings: `ChurchStore.Readings()` with `ReadingRepo`, and `ReadingUsage` ([07 §6](docs/impl/07-readings.md#6-ports));
  - `domain.Reference` and `app.BibleText` are defined ([07 §2](docs/impl/07-readings.md#2-references), [§3.1](docs/impl/07-readings.md#31-lookup-order-and-storing-provider-text-p-50));
  - `BibleTextProvider` gains `ID() string`; `server.WithBibleTextProvider(p)` appends a provider;
  - import: `ChurchStore.Imports()` with `ImportRepo` and `SongRepo.FindDuplicate` ([08 §7](docs/impl/08-import.md#7-ports)); `app.ImportHint` and `app.ImportCandidate` are defined, and `Importer.Parse` reports an unreadable song with `ImportCandidate.Reject`; `server.WithImporter(format, imp)` adds or replaces an importer.
- Step 3, slice 3A:
  - planning setup: `ChurchStore.Duties()` and `.SingingParts()` (`NameListRepo`), `.Templates()` (`TemplateRepo`), `.Services()` (`ServiceRepo`) and `.Seeds()` (`SeedRepo`); `PlanningUsage` answers whether liturgies use a duty or a singing part (`NeverUsed` until slice 3B) ([09 §6](docs/impl/09-planning.md#6-ports));
  - `app.Seed` creates the editable defaults at start-up for churches without the `step3` marker; `Setup` seeds a new church in its own transaction ([09 §3](docs/impl/09-planning.md#3-seeded-defaults-p-56)).
- Step 3, slice 3B:
  - liturgies: `ChurchStore.Liturgies()` (`LiturgyRepo`), `.LiturgyItems()` (`ItemRepo`), `.Assignments()` (`AssignmentRepo`) and `.Edits()` (`EditRepo`) ([10 §9](docs/impl/10-liturgy.md#9-ports));
  - **breaking:** `SongUsage`, `ReadingUsage`, `PlanningUsage` and `NeverUsed` are removed. `ChurchStore.Usage()` (`UsageRepo`) answers `SongInUse`, `SectionsInUse`, `ReadingInUse`, `DutyInUse` and `SingingPartInUse` inside the caller's transaction, so a check under `LockChurch` cannot go stale; `app.Songs`, `app.Readings` and `app.Vocabulary` lose their `Usage` field ([10 §6](docs/impl/10-liturgy.md#6-usage-ports-and-deleted-songs-readings-sections));
  - `app.Liturgies` is new and takes `Entitlements` for `max_active_liturgies` and `max_unpublished_liturgies`;
  - `app.VersionConflictError` (scope `liturgy` or `item`) satisfies `errors.Is(err, ErrVersionConflict)`; new errors `ErrLiturgyLocked`, `ErrLiturgyNotDeletable`, `ErrAssignmentExists`, `LiturgyExistsError`;
  - `PATCH /songs/{id}` with a section list and `DELETE /readings/{id}` now take `LockChurch`.
- Step 3, slice 3D: `EditRepo` gains `BySeq`, `LastActing`, `Newest`, `NewestUndone`, `Foreign`, `SetStatus`, `MarkSkipped` and `DropUndone` for undo and redo ([11 §7](docs/impl/11-liturgy-editor.md)).
- Step 4:
  - **breaking for other implementations:** `ChurchStore` gains `StateChanges()` (`StateChangeRepo`) and `Comments()` (`CommentRepo`); `LiturgyRepo` gains `Transition` and `LockForComment` ([12 §5](docs/impl/12-review.md), [§4](docs/impl/12-review.md));
  - **changed meaning:** `LiturgyRepo.NextSeq` matches only an editable liturgy and returns `ErrNoSeq` when the liturgy is gone or locked ([12 §2](docs/impl/12-review.md)).
- Step 5:
  - **breaking for other implementations:** `ChurchStore` gains `Published()` (`PublishedRepo`, with `PublishedFilter`, `PublishedRow` and `PublishedUpcoming`); `LiturgyRepo` gains `Archive` and `Unarchive`; `LiturgyRow` gains `HasVersions` ([13 §3](docs/impl/13-publishing.md));
  - **changed meaning:** the zero value of the new `LiturgyFilter.Archived` (`ArchivedFilter`) lists liturgies that are not archived; `LiturgyRepo.Delete` refuses a published liturgy or one that has a version (`ErrNotFound`, or `ErrReferenced` when a version appeared after the check) ([13 §2](docs/impl/13-publishing.md), [§5](docs/impl/13-publishing.md)).
- Step 6: new ports `app.SystemInfo` (`Facts`, `OpenBackup`) and `app.BackupStream` for the system page, implemented in package `server`; new errors `ErrStorageFull` (507 `storage_full`), `ErrBackupRunning` (409) and `ErrNotSupported` (400) ([14](docs/impl/14-self-host.md)). The repositories are unchanged.

