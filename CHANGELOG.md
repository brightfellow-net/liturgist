# Changelog

Before v1.0 any port (every interface in `app`, and `httpapi.TenantResolver`) may change. Every change to a port's signature or meaning is listed under **Ports** for its release ([04 §7](docs/impl/04-tenancy-extensions.md#7-extension-points)).

## Unreleased

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
