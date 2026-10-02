# 02 — Persistence (Implementation)

> **Document type: Implementation.** Step 1 of [SPEC.md §10](../SPEC.md#10-suggested-build-order).
> Status: **Approved** 2026-10-02. Items marked **[P-xx]** are decisions listed in the [index](README.md#4-proposed-decisions).

## 1. Scope

The shared SQL adapter (`adapters/sqlstore`), its dialects for SQLite and PostgreSQL, the transaction port, how Go types map to columns, migrations, and the testing strategy. Tables are listed in [reference/schema.md](../reference/schema.md).

## 2. Ports (in `app`)

```go
package app

// Tx runs fn inside one database transaction. fn must not keep s after returning.
type Tx interface {
    Read(ctx context.Context, fn func(s Store) error) error  // read-only transaction
    Write(ctx context.Context, fn func(s Store) error) error // read-write transaction, serialised per church where needed
}

// Store gives access to repositories inside a transaction.
type Store interface {
    // Platform-level repositories (no church scope).
    Users() UserRepo
    Sessions() SessionRepo
    PasswordResets() PasswordResetRepo
    AuthThrottle() ThrottleRepo
    Churches() ChurchRepo           // create/load/count churches; used by setup and TenantResolver
    Translations() TranslationRepo
    SetupTokens() SetupTokenRepo
    InviteTokens() InviteTokenRepo  // find and claim invites by token; the church comes from the invite

    // Serialising locks for platform-level "check, then write" rules (§2.1).
    LockInstall(ctx context.Context) error                 // setup, setup-link issuing
    LockUser(ctx context.Context, id domain.UserID) error  // creating reset links

    // ForChurch returns repositories scoped to one church. Every query they run
    // filters by churchID. On PostgreSQL it also sets the row-level-security
    // setting for the rest of the transaction (§7).
    ForChurch(ctx context.Context, churchID domain.ChurchID) (ChurchStore, error)
}

type ChurchStore interface {
    ChurchID() domain.ChurchID
    Church() ChurchSettingsRepo     // the scoped church row itself
    Memberships() MembershipRepo
    Roles() RoleRepo
    Invites() InviteRepo
    Songs() SongRepo                // songs, sections, groups, search, reindex (06, step 2)
    Readings() ReadingRepo          // (07, step 2)
    Imports() ImportRepo            // batches and candidates (08, step 2)
    LockChurch(ctx context.Context) error // serialise check-then-write rules per church (§3)
}

// Liturgy-use checks for deleting songs, sections and readings (06 §6, 07 §6). Step 2 passes
// app.NeverUsed, which answers "unused"; step 3 replaces it with a query over the liturgies.
type SongUsage interface {
    SongInUse(ctx context.Context, church domain.ChurchID, song domain.SongID) (bool, error)
    SectionsInUse(ctx context.Context, church domain.ChurchID, song domain.SongID, sections []domain.SectionID) ([]domain.SectionID, error)
}
type ReadingUsage interface {
    ReadingInUse(ctx context.Context, church domain.ChurchID, reading domain.ReadingID) (bool, error)
}

type Clock interface{ Now() time.Time }               // always UTC, truncated to microseconds (§4)
type IDGenerator interface{ NewID() string }          // ULID, monotonic within a process
```

- Platform-level questions that cross churches go through `Users()`, never through `ForChurch`: `Users().MembershipChurchIDs(ctx, userID)` (used by `GET /me`, the admin-reset rule in [03 §9](03-identity-auth.md#9-password-reset), and accepting invites as an existing user).
- Use cases never see `*sql.DB`, `*sql.Tx` or SQL.
- Repository methods take and return `domain` types. Repository interfaces are defined in `app`, next to the use cases that need them.
- **Retries (the only authoritative list):** `Tx.Write` re-runs the whole `fn` when the driver reports PostgreSQL `40001` (serialisation failure) or `40P01` (deadlock), or SQLite `SQLITE_BUSY` after the busy timeout. At most **3 attempts**, waiting 10 ms after the first and 50 ms after the second (each ±50 % jitter); waiting stops immediately if `ctx` is cancelled. After the last failed attempt the error becomes `app.ErrUnavailable` → 503 `unavailable`. `Tx.Read` retries the same errors the same way.
- **Retry-safe use cases:** `fn` must have no side effects outside the transaction (no network calls, no sent messages, no cookies). IDs and timestamps may be generated inside `fn`; only values from the **committed** attempt are ever returned, logged or put in cookies, so values from failed attempts are never visible.
- **Deadlines:** `Tx.Write` and `Tx.Read` derive a context with a **10-second** deadline from the caller's; when it expires or the request is cancelled, the transaction rolls back. PostgreSQL connections set `statement_timeout = 5s` and `idle_in_transaction_session_timeout = 15s`. Expensive work (password hashing) is never done inside a transaction ([03 §5](03-identity-auth.md#5-login-and-throttling)).

### 2.1 Atomic operations

Every "check, then write" rule must be safe when two transactions run at once. On SQLite, writes are already one at a time; on PostgreSQL (read committed) they are not. Each rule uses one of these patterns, identically on both databases:

| Pattern | How | Used by |
|---|---|---|
| **Atomic claim** | One conditional statement, e.g. `UPDATE invites SET accepted_at = $now, accepted_user_id = $u WHERE token_hash = $h AND accepted_at IS NULL AND cancelled_at IS NULL AND expires_at > $now RETURNING …`; or `DELETE … WHERE … RETURNING`. Continue only if **exactly 1 row** changed; otherwise the token is treated as already used/expired. The claim is the first write in the transaction, and everything else (users, memberships, sessions) happens in the same transaction. When an invite is accepted by a new user, that user doesn't exist yet at the claim: `accepted_user_id` stays null and is set later in the same transaction | Invite acceptance, reset-link use, setup-token use |
| **Atomic counter** | One `INSERT … ON CONFLICT (key) DO UPDATE SET …` that computes the new count, window start and lock in SQL and returns them (`RETURNING`) | Login throttling ([03 §5](03-identity-auth.md#5-login-and-throttling)) |
| **Conditional update** | `UPDATE … WHERE <key> AND expires_at > $now`; 0 rows changed means the row was revoked or expired | Session extension ([03 §4](03-identity-auth.md#4-sessions)) |
| **Serialising lock** | `LockChurch` (church row), `LockUser` (user row) or `LockInstall` (PostgreSQL `pg_advisory_xact_lock(7420116)`), taken as the first statement; then check; then write | Limits, role safeguards, invite creation (`LockChurch`); reset-link creation (`LockUser`); setup, setup-link issuing (`LockInstall`) |
| **Database constraint** | Partial unique indexes and checks in the [schema](../reference/schema.md) back up the patterns above | One open reset link per user, one open invite per identifier, one setup token, invite state |

Both SQLite (3.35+) and PostgreSQL support `ON CONFLICT … DO UPDATE` and `RETURNING`; the dialect layer only changes placeholders.

## 3. Connections and transactions

**SQLite [P-09]**

| Pool | `MaxOpenConns` | DSN parameters (modernc.org/sqlite) | Used by |
|---|---|---|---|
| Writer | 1 | `_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)&_txlock=immediate` | `Tx.Write`, migrations |
| Readers | 4 | `_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=query_only(1)` | `Tx.Read` |

- File: `<DataDir>/liturgist.db`.
- Because the writer pool has one connection and uses `BEGIN IMMEDIATE`, writes are serialised; `LockChurch`, `LockUser` and `LockInstall` are no-ops on SQLite.

**PostgreSQL [P-09]**

- One pool through `pgx/v5/stdlib`, `MaxOpenConns=10`, `MaxIdleConns=5`, `ConnMaxLifetime=30m`.
- Pool sizes come from `server.Config` (`DBMaxConns`, default 10 for PostgreSQL; `DBMaxReaders`, default 4 for SQLite readers). They are not environment variables in step 1; the SaaS sets them in code.
- `Tx.Read`: `BEGIN READ ONLY` (isolation read committed). `Tx.Write`: read committed.
- `LockChurch`: `SELECT id FROM churches WHERE id = $1 FOR UPDATE` — serialises every "check, then write" rule per church: limit checks (invites, later liturgies) and the role safeguards ([03 §8](03-identity-auth.md#8-member-roles-and-permissions)). Must be the first statement after `ForChurch` in those use cases.

## 4. Type mapping

**[P-10]**

| Go / domain | SQLite (STRICT tables) | PostgreSQL | Notes |
|---|---|---|---|
| Typed IDs (`domain.ChurchID`, `UserID`, `RoleID`, …; ULID strings) | `TEXT` with `CHECK (length(id) = 26)` | `text` with the same check | Generated by `IDGenerator`, never by the database |
| `time.Time` (UTC) | `TEXT`, format `2006-01-02T15:04:05.000000Z` | `timestamptz` | Fixed width so SQLite text sorts correctly; adapter converts both ways. **Canonical precision is microseconds** (PostgreSQL's): `Clock.Now()` returns UTC truncated to µs, and every computed time (expiry = now + duration) is truncated to µs before use, so comparisons give the same result on both databases |
| `*time.Time` | same, `NULL` allowed | same | |
| `bool` | `INTEGER` with `CHECK (x IN (0,1))` | `boolean` | |
| enum (`domain.MemberRole` etc.) | `TEXT` with `CHECK (x IN (...))` | `text` with the same check | Same check text in both migrations |
| JSON object (settings, preferences) | `TEXT` with `CHECK (json_valid(x))` | `jsonb` | Adapter marshals Go structs and validates the shape on every write; unknown keys are preserved; equality is semantic (PostgreSQL may reorder keys) |
| short string | `TEXT` | `text` | Lengths are validated in `domain`, not by column types |
| SHA-256 token hash | `TEXT` with the token-hash check | `text` with the token-hash check | 64 lower-case hex chars, enforced by both databases ([schema conventions](../reference/schema.md#conventions)) |

- Every `CREATE TABLE` in SQLite ends with `STRICT`.
- No database defaults for IDs or timestamps; the app always supplies them (via `IDGenerator`, `Clock`).

## 5. Migrations

- goose v3 as a library; migrations embedded with `//go:embed` from `migrations/sqlite` and `migrations/postgres`.
- File names: `NNNNN_short_name.sql` (5 digits). **Both folders must contain the same version numbers**; a unit test (TC-P-007) checks this. Step 2 adds `00002_library.sql` (songs, readings, imports, [reference/schema.md](../reference/schema.md#step-2-tables)); the SQLite file also creates the FTS5 table `song_fts`, the PostgreSQL file the `fts` column and its GIN index.
- **Forward-only [P-11]:** files contain only `-- +goose Up`. Rolling back means restoring a backup.
- **Migration lock (SQLite):** `serve` (when migrating) and `liturgist migrate` hold an exclusive OS file lock on `<DataDir>/liturgist.lock` from the version check through the copy, the migrations and the copy cleanup. A second process waits up to 30 s, then exits 1 with "another Liturgist process is migrating this database". PostgreSQL uses the advisory lock below instead.
- **On `serve` start** (when `AutoMigrate`) and on `liturgist migrate`:
  1. Read the current version. If it is **greater** than the newest embedded version → exit code 3 with "Database version N is newer than this program (max M). Install version ≥ X or restore a backup." — unless `--allow-newer-schema` is given (§5.1).
  2. If migrations are pending, the driver is SQLite and the database is not brand new (version > 0): write a **pre-upgrade copy** with `VACUUM INTO '<DataDir>/backups/pre-upgrade-v<current version>-<UTC timestamp>-<8 random hex>.db'`.
     - Before copying, check free space ≥ 1.2 × (database file size + WAL file size). If there isn't enough, or the copy fails for any reason, **skip the copy**, delete any partial file, and log a warning with the needed and free bytes (as [SPEC.md §8.3](../SPEC.md#83-self-host-operations) requires), then continue.
     - **Strict mode:** with `LITURGIST_REQUIRE_PREUPGRADE_COPY=true`, a skipped or failed copy aborts instead: exit 1 with "pre-upgrade copy could not be made; migration not started".
     - After a successful copy, keep the newest 3 pre-upgrade copies (by file name) and delete older ones.
     - PostgreSQL gets no automatic copy (documented `pg_dump` instead).
  3. Apply pending migrations in one goose run.

### 5.1 Expand/contract rule and newer schemas

- **Expand/contract:** a release never removes or renames a column, table or constraint that the previous release still uses. First release: add the new structure and switch the code to it ("expand"). A later release: remove the old structure ("contract"). So the previous program version can always run against the current schema, which allows rolling back the program one version without restoring data, and lets old and new SaaS instances run side by side during a deploy.
- **`--allow-newer-schema`** (flag for `serve` and `migrate status`, off by default): start even if the database version is newer than the program; log a warning naming both versions at every start. It **only** skips the refusal to start. It never runs migrations (there are none to run: the database is ahead), never writes to goose's version table, never takes a pre-upgrade copy, and is not accepted by `liturgist migrate`. For operators rolling back one version under the expand/contract rule. Documented in the operator guide, not in the volunteer install guide.
- PostgreSQL: goose runs while holding `pg_advisory_lock(7420115)` so parallel SaaS instances never migrate at the same time.
- Seed data that every install needs (the `translations` rows, [schema](../reference/schema.md#translations)) is inserted by a migration with fixed IDs, so it is identical everywhere.

## 6. Testing strategy

| Level | Database | How |
|---|---|---|
| Domain unit tests | none | Plain Go tests |
| Use-case tests | SQLite **file in `t.TempDir()`** **[P-12]** | `sqlstoretest.NewSQLite(t)` returns a store on a copy of a migrated template; one per test; runs in parallel |
| Template for SQLite tests | One migrated file per test package | Created once (guarded by `sync.Once`) in a package-level temp folder by `sqlstoretest`; each test copies it (`io.Copy`, ~1 ms) and opens the copy with the production pools; the template folder is removed in `TestMain` |
| Repository contract tests | SQLite always; PostgreSQL when `LITURGIST_TEST_POSTGRES=1` **[P-29]** | `storetest.Run(t, factory)` runs the same suite against each dialect |
| Harness clean-up | Packages using `sqlstoretest` call it from `TestMain`: `func TestMain(m *testing.M) { os.Exit(sqlstoretest.Main(m)) }`, which removes the SQLite template and stops the container | — |
| PostgreSQL provisioning | testcontainers-go, image `postgres:17-alpine` | One container per `go test` package run; each test gets a fresh database created from a migrated template (`CREATE DATABASE t_x TEMPLATE liturgist_template`) |

- A temp file (not `:memory:`) is used so tests exercise the same two pools, WAL mode and locking as production. In-memory databases are per connection (writer and readers would see different data), sharing them needs SQLite's shared-cache mode with table-level locks, and they can't use WAL. **[P-12]** amends the decisions-log wording "in-memory".
- CI always sets `LITURGIST_TEST_POSTGRES=1`. Locally, `make test` skips PostgreSQL tests with a visible `t.Skip` message.

## 7. Row-level security hook

- `ForChurch` on PostgreSQL runs `SELECT set_config('liturgist.church_id', $1, true)` (transaction-local) before returning the `ChurchStore`.
- Step 1 creates **no policies**; they are added in `liturgist-saas` ([SPEC.md §8.1.1](../SPEC.md#811-saas-multi-church-hosting-moves-to-liturgist-saas)).
- On SQLite `ForChurch` only records the church ID used by every scoped query.

## 8. Error mapping

The dialect translates driver errors into `app` errors:

| Driver condition | SQLite | PostgreSQL | `app` error |
|---|---|---|---|
| Unique violation | `SQLITE_CONSTRAINT_UNIQUE` / `_PRIMARYKEY` | `23505` | `app.ErrUnique{Constraint}` — the use case maps it to a specific code (e.g. `identifier_taken`) |
| Foreign key violation | `SQLITE_CONSTRAINT_FOREIGNKEY` | `23503` | `app.ErrReferenced` |
| Check violation | `SQLITE_CONSTRAINT_CHECK` | `23514` | `app.ErrInvalid` (a bug: domain validation should have caught it) |
| No rows | `sql.ErrNoRows` | `sql.ErrNoRows` | `app.ErrNotFound` |
| Busy / serialisation | `SQLITE_BUSY` after timeout | `40001`, `40P01` | retried by `Tx.Write` (3×), then `app.ErrUnavailable` |
| Connection failure | open/IO error | connection error | `app.ErrUnavailable` |

Constraint names are identical in both migrations (e.g. `users_email_key`) so mapping by name works for both. PostgreSQL reports the constraint name directly. SQLite reports the **columns** for ordinary unique constraints (`UNIQUE constraint failed: users.email`), so the SQLite dialect keeps a small table from columns to constraint names, filled in with each migration that adds a unique constraint; this includes unique indexes, partial or not (only indexes on expressions are reported by name). When one write violates **several** unique constraints at once, the dialects may report different ones: use cases must not depend on which name comes back in that case, and the name-completeness test (TC-P-010) violates exactly one constraint per case.

## 9. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Write `WHERE church_id = ?` in handlers or use cases | Get repositories from `ForChurch` | Scoped access is the main tenancy safeguard ([SPEC.md §8.1](../SPEC.md#81-tenancy) rule 3) |
| Let the database generate IDs or `now()` timestamps | `IDGenerator` and `Clock` from the app | Same values on both databases; testable time |
| Use `time.Local` or store non-UTC times | Convert to UTC before storing | Mixed zones break sorting and comparisons |
| Write dialect `if` statements inside repositories | Put the difference in the `Dialect` interface | One place for database differences (decisions log) |
| Use `:memory:` SQLite or mock repositories in use-case tests | Temp-file SQLite via `sqlstoretest` **[P-12]** | Tests must hit real SQL, pools and locks |
| Add a migration to only one folder | Add both with the same version number | Test TC-P-007 fails otherwise; databases drift |
| Hold a `Store` after `fn` returns, or do network calls or password hashing inside a transaction | Finish database work inside `fn`; hash and call outside services before or after | Retries rerun `fn`; long transactions block SQLite's single writer and PostgreSQL connections |
| Check a token or count with `SELECT`, then write in a separate statement | Use an atomic pattern from §2.1 | PostgreSQL read committed lets two transactions pass the same check |
| Edit an already released migration | Add a new migration | Installed databases have already run the old one |
| Drop or rename a column in the same release that stops using it | Expand/contract over two releases (§5.1) | The previous version must keep working on the new schema |

## 10. Test case specifications

### Unit tests

| Test ID | Component | Input | Expected output | Edge cases |
|---|---|---|---|---|
| TC-P-001 | Time conversion | `time.Date(2026,10,4,7,0,0,123456789,WIB)` | Stored `2026-10-04T00:00:00.123456Z` (SQLite); reads back equal to the UTC value truncated to µs | Zero time rejected |
| TC-P-002 | `Dialect.Rebind` | `SELECT ? , ?` | SQLite unchanged; PostgreSQL `SELECT $1 , $2` | `?` inside a string literal is not supported — documented, not handled |
| TC-P-003 | Error mapping | Insert duplicate email | `app.ErrUnique{Constraint:"users_email_key"}` on both dialects | Duplicate primary key |
| TC-P-004 | `Tx.Write` retry | `fn` returns a busy error twice, then succeeds | `fn` called 3 times; result committed; values returned are from the 3rd attempt | 3rd failure → `ErrUnavailable`; `40P01` retried like `40001`; cancelled context stops waiting immediately |
| TC-P-009 | Time precision | `Clock.Now()` with nanoseconds; expiry `now + 24h` | Both truncated to µs; a token expiring at exactly `now` is expired on both dialects | — |
| TC-P-005 | Migrations | Fresh database | Version = newest embedded | Running twice is a no-op |
| TC-P-006 | Downgrade protection | Database version set to newest+1 | `serve` exits with code 3 and the message in §5 | With `--allow-newer-schema`: starts and logs a warning |
| TC-P-008 | Pre-upgrade copy | SQLite at version N with pending migrations | `backups/pre-upgrade-vN-*.db` exists, opens, and has version N; only the newest 3 copies kept | Not enough free space (database + WAL) → copy skipped with warning, migrations still applied; same with strict mode → exit 1, no migration; no pending migrations → no copy; a second process migrating at the same time waits for the lock file |
| TC-P-007 | Migration sets | Embedded folder listings | Same version numbers in `sqlite/` and `postgres/` | Extra file in one folder fails |

### Integration (contract) tests — run against both dialects

| Test ID | Flow | Setup | Verification | Teardown |
|---|---|---|---|---|
| IT-P-001 | Scoped access | Two churches A and B (inserted directly), one invite each | `ForChurch(A).Invites().List()` returns only A's invite | Drop test database |
| IT-P-002 | Composite foreign keys | Membership in A | Inserting a membership role with church B's ID and A's membership fails with `ErrReferenced` | — |
| IT-P-003 | Limit lock | Two concurrent `Tx.Write` calls that count invites and insert one, limit 1 | Exactly one succeeds; the other sees the new count | — |
| IT-P-004 | Read-only pool | `Tx.Read` attempting an insert | Error; nothing written | — |
| IT-P-005 | RLS hook (PostgreSQL only) | `ForChurch(A)` | `current_setting('liturgist.church_id', true)` = A inside the transaction, empty after commit | — |
| IT-P-006 | Seeded translations | Fresh database | 6 rows with the fixed IDs and codes in the schema reference | — |
| IT-P-007 | Scoped repository coverage | Two churches A and B with rows in every church-owned table | **Every** method of every `ChurchStore` repository, called through `ForChurch(A)`, reads and changes only A's rows. A table-driven harness lists all methods; adding a method without adding it to the harness fails a reflection check | — |
| IT-P-008 | Race harness | Two transactions synchronised by a barrier after their first read | For each atomic pattern in §2.1, exactly one transaction succeeds and the other observes it; run on both dialects | — |
| IT-P-009 | Database constraints | Direct inserts bypassing the app | Second open reset link per user, second open invite for the same email or phone, a second setup-token row, an accepted-and-cancelled invite, a malformed token hash and a malformed phone are all rejected on both dialects | — |

## 11. Error handling matrix

| Error | Detection | Response | Fallback | Logging |
|---|---|---|---|---|
| Database file locked by another process | `SQLITE_BUSY` at start-up migration | Exit 1: "database is in use by another process" | — | error |
| Disk full during write | `SQLITE_FULL` / PostgreSQL `53100` | `ErrUnavailable` → 503 `unavailable` (step 6 adds the "storage full" message) | — | error |
| Migration fails half-way | goose error | Exit 1; SQLite migration runs in a transaction so nothing partial remains | Restore the pre-upgrade copy | error with version |
| No space for pre-upgrade copy | Free-space check | Skip the copy, continue migrating | — | warn with needed and free bytes |
| Database newer than binary | Version compare | Exit 3 | Install newer version | error |
| Serialisation conflict or deadlock persists | 3 attempts exhausted | 503 `unavailable` | Client may retry | warn |
| Transaction deadline exceeded | 10 s context deadline / PostgreSQL `statement_timeout` | Roll back; 503 `unavailable` | Client may retry | warn with operation name |
| Another process is migrating | Lock file held > 30 s | Exit 1 | Retry after the other process finishes | error |
| Pre-upgrade copy impossible in strict mode | Free-space check or copy error | Exit 1, no migration | Free disk space | error |

## 12. References

| Topic | Location |
|---|---|
| Shared SQL adapter, dialect layer (decision) | [SPEC.md §11](../SPEC.md#11-decisions-log) — row "One shared SQL persistence adapter" |
| Use-case and contract tests (decision) | [SPEC.md §11](../SPEC.md#11-decisions-log) — row "Use-case tests run against the real SQLite adapter" |
| SaaS database layout, row-level security (decision) | [SPEC.md §11](../SPEC.md#11-decisions-log) — rows "SaaS database layout", "PostgreSQL row-level security" |
| Tables and constraints | [reference/schema.md](../reference/schema.md) |
| Tenancy rules | [SPEC.md §8.1](../SPEC.md#81-tenancy) |
| Upgrades and pre-upgrade copy | [SPEC.md §8.3](../SPEC.md#83-self-host-operations), §5 above |
