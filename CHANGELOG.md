# Changelog

Before v1.0 any port (every interface in `app`, and `httpapi.TenantResolver`) may change. Every change to a port's signature or meaning is listed under **Ports** for its release ([04 §7](docs/impl/04-tenancy-extensions.md#7-extension-points)).

## Unreleased

### Ports

- First definitions, all provisional [P-27]:
  - persistence: `Tx`, `Store`, `ChurchStore` and their repositories, `Clock`, `IDGenerator` ([02 §2](docs/impl/02-persistence.md#2-ports-in-app));
  - identity: `PasswordHasher` ([03 §3](docs/impl/03-identity-auth.md#3-passwords));
  - tenancy: `httpapi.TenantResolver`, `URLBuilder` ([04 §3–§4](docs/impl/04-tenancy-extensions.md#3-tenantresolver));
  - extension points: `Entitlements`, `AuthProvider`, `Storage`, `EventBus`, `Notifier`, `BibleTextProvider`, `Exporter`, `Importer` ([04 §7](docs/impl/04-tenancy-extensions.md#7-extension-points)).
- Placeholder types without fields, designed in the step that first uses them: `domain.Reference`, `app.BibleText`, `app.NotifyEvent`, `app.Message`, `app.PublishedVersion`, `app.ImportHint`, `app.ImportCandidate`.
- `AuthProvider` is defined but not used yet: password login stays in `app.Auth` until a second provider arrives.
