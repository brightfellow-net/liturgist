# 14 — Self-Host Packaging: Backup and Restore, System Page, Releases (Implementation)

> **Document type: Implementation.** Step 6 of [SPEC.md §10](../SPEC.md#10-suggested-build-order), implementing [SPEC §8.3](../SPEC.md#83-self-hosting-operations), in slices 6A to 6F.
> Status: decisions **[H-1] to [H-14] Approved** 2026-10-05, each as proposed (the owner approved all recommendations). Spec Gate and adversarial review have not run; slices are drafted one at a time and each waits for the owner's approval before merging.

## 1. Scope and what already exists

Checked against the code on 2026-10-05 (`main` at `172bab6`).

| SPEC §8.3 item | State | Where |
|---|---|---|
| Migrations on startup, pre-upgrade copy, keep newest 3 | **Built** (step 1). Name is `backups/pre-upgrade-vNNNNN-<UTC>-<rand>.db`, not `pre-upgrade-<old version>.db` as SPEC says | `adapters/sqlstore/migrate.go` |
| Downgrade protection, `--allow-newer-schema`, `migrate`, `migrate status`, `LITURGIST_REQUIRE_PREUPGRADE_COPY` | **Built** | `server/config.go`, `cmd/liturgist/main.go` |
| `/healthz`, `/readyz` (database reachable, migrations applied) | **Built** | `server/http.go:47` |
| `slog` logs with request ID, no query strings | **Built** | `server/http.go` |
| `liturgist version` | **Built** | `cmd/liturgist/main.go` |
| `files/` storage folder | **Built** (`localfs.Storage`, nothing writes to it yet) | `server/server.go:142` |
| "disk full" on a failed SQLite write | **Partly**: `SQLITE_FULL` maps to `ErrUnavailable`; no user message, no PostgreSQL case | `adapters/sqlstore/sqlite.go:104` |
| CI | **Built** (tests only) | `.github/workflows/ci.yml` |
| `liturgist backup`, `restore`, scheduled backups, rotation | **Missing** | — |
| Church admin system page, "Download backup", disk and backup warnings, update check | **Missing** | — |
| Built-in HTTPS (`LITURGIST_DOMAIN`, `certmagic`) | **Missing** | — |
| Release builds, Docker image, Windows service, systemd unit, signed checksums, `liturgist healthcheck` | **Missing** | — |
| Install, reverse-proxy and off-site backup guides, README | **Missing** (`README.md` exists; not checked for step 6 content) | — |

| Left out | Why |
|---|---|
| Built-in off-site upload (S3 and others) | SPEC §8.3: documented (rclone, Litestream), not built in |
| PostgreSQL backup and restore commands | SPEC §8.3: documented `pg_dump` steps; `backup` and `restore` refuse on PostgreSQL and point to the guide |
| Any system page or backup download in the hosted edition | One download would contain every church ([H-8](#2-decisions-proposed)) |
| Automatic update installation | SPEC §8.3: the update check only shows "update available" |

**Pilot gate.** [PILOT.md §4](../PILOT.md) needs a backup restore tested once on the pilot instance. Slice 6A is that gate; 6B to 6F are not.

## 2. Decisions (Approved 2026-10-05, as proposed)

| ID | Decision | Alternatives considered |
|---|---|---|
| H-1 | **Archive format** is one `.zip` (opens on any desktop, stdlib `archive/zip`): `manifest.json`, `liturgist.db`, `files/…`. The manifest holds `format: 1`, program version, schema version, `created_at` (UTC), driver `sqlite`, and the SHA-256 of `liturgist.db` | zip or tar.gz |
| H-2 | **`serve` holds a run lock** (`liturgist.run`, flock, non-blocking) for its whole life (taken by the `serve` command through `server.LockRun`, not by `server.New`, [§14](#14-slice-6a-as-built-2026-10-05)). A second `serve` on the same data folder refuses to start; `restore` refuses while it is held. This is how "server stopped" is checked, and it also stops two servers corrupting one folder | yes / no |
| H-3 | **Scheduler runs inside `serve`**, not cron. Daily at `LITURGIST_BACKUP_TIME` (default `02:00`, in the church's time zone, UTC before setup); `LITURGIST_BACKUP_TIME=off` turns it off. A missed run (server off at 02:00) runs 5 minutes after start when the newest automatic backup is over 26 hours old | in-process / cron docs only |
| H-4 | **Retention** `LITURGIST_BACKUP_KEEP_DAILY=7`, `LITURGIST_BACKUP_KEEP_WEEKLY=4`: keep the newest 7 automatic backups, plus the newest one of each of the 4 ISO weeks before them. Only `backups/auto-*.zip` is ever pruned; manual, `pre-upgrade-*` and `pre-restore-*` files are never touched by this | numbers |
| H-5 | **"Last backup" warning** on the system page and as a banner for church admins when the newest backup of any kind is over 48 hours old, **or** no backup was downloaded or written by `liturgist backup` to another path in 30 days (a copy on the same disk does not survive the disk) | thresholds |
| H-6 | **Disk-space warning** below 1 GiB free or 5% free, whichever is larger. A backup or pre-upgrade copy needs `1.2 ×` the database size plus the size of `files/` free, else it is skipped with a warning (a manual `backup` refuses with exit 1) | numbers |
| H-7 | **Decided 2026-10-05: build now** (owner approved the `certmagic` dependency when 6F started). Original proposal: **Built-in HTTPS (`certmagic`) is deferred** to slice 6F, after the pilot. It is a large new dependency and the pilot runs behind Caddy or a tunnel (guides in 6E). Needs your approval of the dependency when 6F starts | defer / build now |
| H-8 | **System routes exist only in the single-church (community) tenancy.** The hosted edition does not register them, and `TestTenancyDeclarations` names each as community-only | yes / no |
| H-9 | **Gate** for the system page and the download is `church.settings` ([domain/scope.go](../../domain/scope.go)), held by the Church admin role | scope |
| H-10 | **Windows service** with `golang.org/x/sys/windows/svc` (`x/sys` is already in `go.mod`): `liturgist service install\|uninstall\|start\|stop`. No new module | yes / defer |
| H-11 | **Docker health check** is a new subcommand `liturgist healthcheck` (GET `/healthz` on the listen address, exit 0 or 1), because the image is distroless and has no `curl` | yes / no |
| H-12 | **Release tooling:** GoReleaser run by a GitHub Actions workflow on `v*` tags; builds linux amd64 and arm64, windows amd64, a multi-arch image on `ghcr.io/brightfellow-net/liturgist`, and `checksums.txt`. **Signing:** cosign keyless (GitHub OIDC; no key for anyone to lose) over `checksums.txt` | cosign / minisign / GPG |
| H-13 | **Update check** (off by default, `LITURGIST_UPDATE_CHECK=true`): the server asks the GitHub releases API for the newest tag once a day and sends nothing but the request. The system page shows "update available" | build / defer |
| H-14 | **Backups contain password hashes and sessions.** The download is audit-logged (`backup_downloaded`, actor), sent `Cache-Control: no-store`, and the guide says to treat the file like the database. Restore keeps sessions as they were | yes / no |

## 3. `liturgist backup` and `restore` (slice 6A) [H-1, H-2]

### 3.1 `liturgist backup [file]`

- **Default path** `<data dir>/backups/manual-<UTC yyyymmddThhmmssZ>.zip`; `file` may be any path (a USB stick, a mounted share). An existing file is never overwritten (exit 1).
- **SQLite only.** On PostgreSQL: exit 1 with "use pg_dump; see the backup guide".
- **Runs while the server runs.** Steps: (1) check free space (H-6) at the target; (2) `VACUUM INTO` a temp file in the target's folder, through a read connection (a consistent snapshot, no lock on writers beyond WAL's); (3) `PRAGMA integrity_check` on the temp file; (4) write the zip to `<file>.partial`: manifest, `liturgist.db`, then `files/` (database first, so every file the snapshot refers to still exists unless deleted since: a vanished file is skipped and logged as a warning); (5) fsync, rename to `<file>`; (6) remove temps. Any failure removes the temps and the partial file.
- **Output** one line: path, size, schema version. Exit codes as 01 §6 (0 ok, 1 error, 2 usage).
- `ErrUnavailable`/`ENOSPC` while writing: exit 1 "server storage is full" (§6).

### 3.2 `liturgist restore <file> [--yes]`

Runs with the server stopped (H-2). Order, each step before the next:

| # | Step | On failure |
|---|---|---|
| 1 | Take `liturgist.run` and `liturgist.lock`; if held: "stop the server first" | exit 1, nothing changed |
| 2 | Open the zip; read `manifest.json`; `format` known, driver `sqlite`, SHA-256 of `liturgist.db` matches | exit 1, nothing changed |
| 3 | Schema version of the backup greater than this program's: refuse, exit 3 (as `serve`); smaller: allowed, the next `serve` migrates it after its own pre-upgrade copy | exit 3 |
| 4 | Extract to `<data dir>/restore-tmp/`; `PRAGMA integrity_check` and `PRAGMA foreign_key_check` on the extracted database; free space ≥ the archive's uncompressed size + `1.2 ×` the current database | exit 1, tmp removed |
| 5 | Ask "Replace the current data? (y/N)"; `--yes` skips it; without a terminal and without `--yes` it refuses | exit 1 |
| 6 | Keep the current data: `VACUUM INTO backups/pre-restore-<UTC>.db`, or, if the current database cannot be opened, rename it to that name; rename the current `files/` to `backups/pre-restore-files-<UTC>/` | exit 1, nothing replaced |
| 7 | Rename the extracted database to `liturgist.db` (removing `-wal` and `-shm` first), the extracted `files/` to `files/`; remove `restore-tmp/` | the `pre-restore-*` copies stay; the message names them |

- A restore never deletes a `pre-restore-*` copy; the guide says to delete them by hand when the church is satisfied.
- **Restore drill** (also the PILOT §4 check): back up a seeded instance, change data, stop, restore, start, compare. It is a test (§9) and a section of the guide.

## 4. Scheduled backups and disk space (slice 6B) [H-3, H-4, H-6]

- **Scheduler** `server/backup_scheduler.go`: started by `Run` on SQLite only, stopped with the server context; one backup at a time (a mutex also shared with the download, §5); a run that fails logs `backup_failed` at ERROR and tries again at the next slot; it does not retry in a loop.
- **Files** `backups/auto-<UTC>.zip`, written like §3.1 steps 2 to 6, then pruned by H-4. Pruning runs only after a successful backup, so a failing disk never deletes the last good copy.
- **Config** `LITURGIST_BACKUP_TIME` (`HH:MM` or `off`), `_KEEP_DAILY`, `_KEEP_WEEKLY`, parsed in `internal/envconfig`, invalid value is a start-up error (exit 2) like the other settings.
- **Disk space** `sqlstore.FreeBytes(dir)` already exists (`freeBytes`, used by the pre-upgrade copy); it is exported for the server. The pre-upgrade copy's `1.2 ×` rule moves to one shared helper with §3.
- **"Server storage is full"**: `ErrUnavailable` from `SQLITE_FULL` (and PostgreSQL class `53100`) maps to **507 `storage_full`**, message "Server storage is full. Ask the person who runs the server to free some space.", in English and Indonesian. The web app shows it as the error of the failed save.

## 5. System page (slice 6C) [H-8, H-9, H-14]

| Route | Scope | Response |
|---|---|---|
| `GET /api/v1/system/status` | `church.settings`, community tenancy only | `{ version, commit, database: { driver, size_bytes, schema_version }, disk: { free_bytes, total_bytes, low }, backup: { last_at, last_kind, last_downloaded_at, warning }, email_configured, https: { mode, base_url_scheme, plain_http_warning }, update: { enabled, latest, available } }` |
| `GET /api/v1/system/backup` | same | the archive of §3.1, streamed, `Content-Type: application/zip`, `Content-Disposition: attachment; filename="liturgist-<church slug>-<date>.zip"`, `Cache-Control: no-store`; takes the backup mutex; 409 `backup_running` if one is in progress; 400 `not_supported` on PostgreSQL |

- **Last copy taken away** (a download, or `liturgist backup` to a path outside `backups/`) is recorded as the empty file `backups/.last-copy` (its mtime); no migration. `last_at` is the newest mtime of `auto-*`, `manual-*` files.
- **HTTPS status** `mode` is `behind_proxy` (trusted proxies configured), `plain_http`, or, from 6F, `built_in`. `plain_http_warning` reuses `StartupWarnings` (`server/config.go`).
- **Web:** page `/settings/system` (church admins; link in the settings menu) with the facts above and the **Download backup** button; a **banner** in the app shell for church admins when `disk.low` or `backup.warning` (dismissable for the session, back on next load). All strings in `en` and `id`.
- **Tenancy:** the two operations are declared community-only in `TestTenancyDeclarations` and are not registered in the hosted build (H-8).
- **Logging:** `backup_downloaded` (actor, size), never the contents.

## 6. Health, logs, errors (slice 6B, small)

`/healthz` and `/readyz` stay as they are. `liturgist healthcheck` (slice 6D, H-11) calls `/healthz` on the configured listen address with a 3-second timeout. Logs already carry a request ID and no personal data; the new events (`backup_written`, `backup_failed`, `backup_pruned`, `backup_downloaded`, `restore_done`) carry paths, sizes and versions only.

## 7. Releases (slice 6D) [H-10 to H-13]

| Piece | Content |
|---|---|
| `Dockerfile` | multi-stage: build the web app, build the binary with `CGO_ENABLED=0`, copy to a distroless static image; `VOLUME /data`, `LITURGIST_DATA_DIR=/data`, `LITURGIST_LISTEN=:8080`, non-root user, `HEALTHCHECK CMD ["/liturgist","healthcheck"]` |
| `.goreleaser.yaml` | `ldflags` set `server.Version`, `Commit`, `BuildDate`; targets linux amd64 and arm64, windows amd64; archives `tar.gz` (zip on Windows) with `README`, `LICENSE`, the systemd unit; `checksums.txt`; the image on ghcr.io (multi-arch) |
| `.github/workflows/release.yml` | on tag `v*`: `make web`, tests, GoReleaser, cosign sign of `checksums.txt` and the image; release notes from `CHANGELOG.md` |
| `deploy/liturgist.service` | systemd unit: dedicated user, `StateDirectory=liturgist`, `LITURGIST_DATA_DIR=/var/lib/liturgist`, `Restart=on-failure`, hardening (`NoNewPrivileges`, `ProtectSystem=strict`, `ReadWritePaths`) |
| `liturgist service …` | Windows only (build tag); other platforms print "not supported" |
| Version tags | semantic, `v0.x` until stable (SPEC §8.3) |

## 8. Guides and README (slice 6E)

`docs/self-host/`: `install-linux.md`, `install-windows.md`, `install-docker.md`, `https-caddy.md`, `https-nginx.md`, `https-cloudflare-tunnel.md`, `https-tailscale.md`, `backup-and-restore.md` (including the restore drill, `rclone` and Litestream off-site copies, PostgreSQL `pg_dump` steps), `upgrading.md` (pre-upgrade copy, `--allow-newer-schema`, downgrading means restoring), `configuration.md` (every `LITURGIST_*` variable). `README.md` gains a short quick start. Each guide's commands are run once on a clean machine or container before the slice is submitted.

## 9. Slices

| Slice | Content | Decisions needed first |
|---|---|---|
| **6A** (**merged** 2026-10-05, [§14](#14-slice-6a-as-built-2026-10-05)) | `serve` run lock; `backup`; `restore`; the zip format; shared free-space helper; `backup-and-restore.md` (first draft) | H-1, H-2, H-6 |
| **6B** (**merged** 2026-10-05, [§15](#15-slice-6b-as-built-2026-10-05)) | Scheduler, retention, config; `storage_full` 507 and its strings | H-3, H-4 |
| **6C** (**merged** 2026-10-05, [§16](#16-slice-6c-as-built-2026-10-05)) | System routes, system page, banners, download | H-5, H-8, H-9, H-14 (+ H-13 if built) |
| **6D** (**merged** 2026-10-05, [§17](#17-slice-6d-as-built-2026-10-05)) | Dockerfile, GoReleaser, release workflow, systemd unit, Windows service, `healthcheck` | H-10, H-11, H-12 |
| **6E** (**merged** 2026-10-05, [§18](#18-slice-6e-as-built-2026-10-05)) | The rest of the guides and the README | none |
| **6F** (**merged** 2026-10-05, [§19](#19-slice-6f-as-built-2026-10-05)) | Built-in HTTPS with `certmagic` (deferred) | H-7 |

Each slice is drafted in a worktree, shown to the owner with its deviations, and merged only on explicit approval. A docs commit with the as-built notes follows each, as in step 5.

## 10. Anti-patterns (DO NOT)

| ❌ Don't | ✅ Do instead | Why |
|---|---|---|
| Copy `liturgist.db` with `cp` or `io.Copy` while the server runs | `VACUUM INTO` a temp file | A copy of a WAL database can be torn or miss committed data |
| Write the final archive in place | Write `<file>.partial`, fsync, rename | A crash leaves a half archive that looks valid |
| Prune old backups before the new one is verified | Prune only after the new one is complete | A full disk would delete the last good copy |
| Restore over the live database without keeping it | Always write `pre-restore-*` first, and never delete it | The operator may restore the wrong file |
| Let `restore` run while `serve` runs | Refuse on the run lock (H-2) | The server would write into the replaced file |
| Trust the archive's paths | Reject entries with `..`, absolute paths or links (zip-slip) | A crafted archive could write outside the data folder |
| Add a backup route to the hosted build | Community tenancy only (H-8) | One download would hold every church |
| Put the backup in a URL or log line with contents | Log path and size only | The file holds password hashes |
| Add the `minimumReleaseAgeExclude` setting to install a new module | Wait for the age, or ask the owner | Standing owner rule |
| Build the update check to send the install's data | Send nothing but the GET | Self-hosters expect no telemetry (SPEC §8.3) |

## 11. Test case specifications

### Unit and package tests (Go, both dialects where the code is shared)

| ID | Component | Input | Expected |
|---|---|---|---|
| TC-601 | zip writer/reader | archive with a `../x` entry, an absolute path, a symlink | refused, nothing written |
| TC-602 | manifest | unknown `format`; wrong SHA-256; driver `postgres`; newer schema | each refused with its own message and exit code (3 for newer schema) |
| TC-603 | `backup` | seeded database with writers running (goroutine inserting) | archive's database passes `integrity_check`; row count between the counts before and after |
| TC-604 | `backup` | target folder with less free space than needed (injected `freeBytes`) | exit 1, no files left |
| TC-605 | `backup` | existing target file | refused, file unchanged |
| TC-606 | retention | 20 `auto-*` files over 6 weeks plus a `manual-*` and a `pre-upgrade-*` | newest 7 + one per previous 4 ISO weeks kept; the other two files untouched |
| TC-607 | scheduler | fake clock past 02:00; past-due start; `off` | one backup; one backup 5 minutes after start; none |
| TC-608 | run lock | second `serve` on the same folder | refuses with a clear message |
| TC-609 | `ENOSPC` | injected write error during backup and during a save | `backup` exit 1 "storage is full"; the API answers 507 `storage_full` |
| TC-610 | system status | disk 0.5 GiB free; last backup 3 days old; no download in 31 days | `disk.low`, `backup.warning` true |

### Integration tests

| ID | Flow | Verification |
|---|---|---|
| IT-601 | **Restore drill**: seed, `backup`, change data, stop, `restore --yes`, start | data equals the seed; `pre-restore-*` holds the changed data; files restored |
| IT-602 | Restore a backup from an older schema version | the next `serve` makes a pre-upgrade copy and migrates |
| IT-603 | `restore` interrupted after step 6 (injected failure) | `pre-restore-*` present; the message names it; a second `restore` of the same file succeeds |
| IT-604 | Download as Church admin, as another member, in the hosted build | 200 zip; 403; route absent (404) |
| IT-605 | Web: system page as admin; low-disk banner | facts shown; banner visible; hidden for non-admins |
| IT-606 | Docker image on a clean machine | `docker run` serves, health check turns healthy, data survives a container restart |

## 12. Error handling matrix

| Condition | Where | Response | Message |
|---|---|---|---|
| Backup target exists | CLI | exit 1 | "That file already exists." |
| Not enough free space | CLI / scheduler / download | exit 1 / skip + WARN / 507 | "Not enough free disk space for a backup." |
| Disk full on a save | API | 507 `storage_full` | "Server storage is full. Ask the person who runs the server to free some space." |
| PostgreSQL | CLI / route | exit 1 / 400 `not_supported` | "Use pg_dump; see the backup guide." |
| Server running during `restore` | CLI | exit 1 | "Stop the server first." |
| Archive damaged or SHA mismatch | CLI | exit 1 | "This backup is damaged and was not restored." |
| Backup newer than the program | CLI | exit 3 | "This backup was made by a newer Liturgist; upgrade the program first." |
| Backup in progress when downloading | API | 409 `backup_running` | "A backup is running. Try again in a minute." |
| Scheduled backup fails | scheduler | ERROR `backup_failed`; next slot | banner via `backup.warning` after 48 hours |

## 13. References

| Topic | Location |
|---|---|
| Self-hosting operations | [SPEC §8.3](../SPEC.md#83-self-hosting-operations) |
| Build order, step 6 | [SPEC §10](../SPEC.md#10-suggested-build-order) |
| Pilot prerequisites (restore test) | [PILOT.md §4](../PILOT.md) |
| Pre-upgrade copy, `allow-newer`, lock file | `adapters/sqlstore/migrate.go`, [02-persistence.md §5](02-persistence.md) |
| CLI, exit codes, configuration | [01-foundation.md §5, §6](01-foundation.md) |
| Scopes | `domain/scope.go`, [03-identity-auth.md §8](03-identity-auth.md#8-member-roles-and-permissions) |
| Tenancy declarations | [04-tenancy-extensions.md](04-tenancy-extensions.md) |

## 14. Slice 6A as built (2026-10-05)

Merged to `main` as `2ac8faf`. Code: `internal/backup` (zip, manifest, entry checks), `adapters/sqlstore/backup.go` (`SnapshotFile`, `CheckSQLiteFile`, `ProgramVersion`, `LockRun`, `FreeBytes`, `SnapshotSpace`), `server/backup.go` (`Backup`, `Restore`), `cmd/liturgist/main.go` (`backup`, `restore`, run lock in `serve`). Guide: [backup-and-restore.md](../self-host/backup-and-restore.md).

| Deviation from §2 and §3 | Why |
|---|---|
| The run lock is taken by the `serve` command (`server.LockRun`), not by `server.New` | Existing tests, and possibly the hosted edition, build several servers on one folder. Behaviour of H-2 for `serve` and `restore` is unchanged |
| Restore's free-space rule is the archive's uncompressed size + `1.2 ×` the current database | "1.2 × the current data" was vague |
| The manifest's SHA-256 covers the database only; `files/` entries rely on the zip's CRC | Per-file checksums were not specified; add if the pilot stores files that matter |
| `SnapshotFile` uses its own connection, without `query_only` | `VACUUM INTO` is refused under `query_only`; a separate connection also never queues behind the server's writer |
| Sessions stay in a restored database (as H-14) | Not changed by restore |

Tests: TC-601, 602, 603, 604, 605, 608, 609 (mapping), IT-601 (server and CLI), IT-603, plus restore over a damaged database, a manifest that understates the schema, and the CLI exit code 3. TC-606, 607, 610 and IT-602, 604 to 606 belong to later slices. Go suite green on SQLite and PostgreSQL; golangci-lint not run.

## 15. Slice 6B as built (2026-10-05)

Merged to `main` as `9184097`. Code: `server/backup_scheduler.go` (scheduler, retention, stale-temp cleanup), `server/config.go` and `internal/envconfig` (`LITURGIST_BACKUP_TIME`, `_KEEP_DAILY`, `_KEEP_WEEKLY`), `app.ErrStorageFull` with its mappings in `adapters/sqlstore` and the 507 problem in `adapters/httpapi/problem.go`, the web messages in `packages/i18n`. Guide: the "Automatic backups" section of [backup-and-restore.md](../self-host/backup-and-restore.md).

| Deviation from §2 and §4 | Why |
|---|---|
| `LITURGIST_BACKUP_KEEP_WEEKLY=0` means no weekly backups; `Config.BackupKeepWeekly` is `-1` for that and `0` for the default 4 | Zero in a hand-built `Config` means "the default" |
| A hand-built `Config{}` has automatic backups off; `02:00` is the default only from the environment | Tests and the hosted edition must not back up by accident |
| Weeks are ISO weeks in the church's time zone | Matches the 02:00 slot |
| A week that already has a kept backup (daily or an earlier weekly pick) keeps nothing more | Makes "the newest one of each of the 4 ISO weeks before them" exact |
| `storage_full` is a new `app.ErrStorageFull`, no longer a kind of `ErrUnavailable`; 507 is added to [01 §10](01-foundation.md); it is not declared per operation in the OpenAPI document | A full disk and a busy database need different messages |
| 6A fix: `storageErr` tested `ErrUnavailable` (also a busy database); it now tests `ErrStorageFull` and `ENOSPC` | Wrong message for a busy database |

Tests: TC-606, TC-607, TC-609 (all parts), the scheduler wiring (community, SQLite, time set), the church time zone lookup, the env settings. TC-610 and IT-602, 604 to 606 belong to later slices. Go suite green on SQLite and PostgreSQL; Vitest 302; golangci-lint not run.

## 16. Slice 6C as built (2026-10-05)

Merged to `main` as `e5b0639`. Code: `app/system.go` (`System`, the `SystemInfo` port), `server/system.go` (`systemInfo`: facts, thresholds, the download stream), `server/updatecheck.go`, `adapters/httpapi/system_ops.go`, `adapters/sqlstore` (`DiskUsage`, `DB.SizeBytes`), web `SystemPage`, `SystemBanners`, the System tab. Guide: "Download a backup from the browser" in [backup-and-restore.md](../self-host/backup-and-restore.md).

| Deviation from §2 and §5 | Why |
|---|---|
| The marker is `backups/.last-copy`, touched by a download and by `liturgist backup` to a path outside `backups/` (H-5 wording: "downloaded or written by `liturgist backup` to another path") | One marker for both; a copy made by hand (rclone) is not seen |
| "No backup" and "no copy" count from the later of the last event and the church's creation date | A new church is not warned in its first 48 hours or 30 days |
| `https.proxy_missing_warning` is added beside `plain_http_warning` | The existing start-up warning has two conditions |
| `last_at` is the newest `auto-*` or `manual-*` file by its name; the "backup" warning is `stale` (over 48 hours) or `not_copied` (over 30 days), `stale` first | Definition left open in §5 |
| The download makes its snapshot before sending; a full disk is 507 `storage_full`, a running backup 409 `backup_running`, PostgreSQL 400 `not_supported` (new codes `backup_running`, `not_supported`, with web messages) | A broken download would not show a message |
| The system routes are registered unless a tenant resolver is given (`deps.hosted`); both operations are declared `church` in `TestTenancyDeclarations` | H-8 |
| `email_configured` is always false | There is no email feature yet |
| The update check starts one minute after start, then daily; a `dev` build is never out of date; a release tag suffix (`-5-gabc`) is ignored in the comparison | H-13 details |

Tests: TC-610, IT-604, IT-605, E2E-W-021, plus the update check, `SizeBytes` on both dialects and the copy marker. IT-602 and IT-606 belong to later slices. Go suite green on SQLite and PostgreSQL; Vitest 315; Playwright 41; golangci-lint not run.

## 17. Slice 6D as built (2026-10-05)

Merged to `main` as `62b9015`. Code: `cmd/liturgist/healthcheck.go`, `envfile.go`, `service_windows.go`, `service_other.go`; `Dockerfile`, `.dockerignore`, `.goreleaser.yaml`, `.github/workflows/release.yml`, `deploy/liturgist.service`, `scripts/release-notes.sh`.

Deviations from §7:

| Plan | As built |
|---|---|
| GoReleaser builds the image | The workflow builds the multi-arch image from the `Dockerfile` with buildx (the Dockerfile cross-compiles, no emulation). GoReleaser makes only the archives and `checksums.txt` |
| Windows service settings | `service run` reads `%ProgramData%\Liturgist\liturgist.env` (`KEY=VALUE`; real environment variables win). `install` writes a commented sample. Default data folder `%ProgramData%\Liturgist\data`; the log is `liturgist.log` there, **not rotated** |
| Release notes | `scripts/release-notes.sh vX.Y.Z` prints the `## X.Y.Z` section of `CHANGELOG.md` and fails if absent |
| Archives | The Linux archive has the systemd unit; the Windows zip does not |

Verified: the image builds (31.7 MB), is healthy, runs as 65532, backs up into the volume and stops with exit 0; `goreleaser check` and a snapshot build; Windows and arm64 cross-builds. **Not verified:** the release workflow, cosign signing, the ghcr push (need a real tag), the Windows service (needs Windows), the unit's hardening under real systemd. Before the first release: try `service install/start/stop/uninstall` on Windows and add a `## 0.1.0` section to `CHANGELOG.md`.

## 18. Slice 6E as built (2026-10-05)

Merged to `main` as the commit after `dfc72da`. Files: `README.md` and, in `docs/self-host/`, `install-docker.md`, `install-linux.md`, `install-windows.md`, `https-caddy.md`, `https-nginx.md`, `https-cloudflare-tunnel.md`, `https-tailscale.md`, `upgrading.md`, `configuration.md`, and a new "Keep a copy somewhere else" section in `backup-and-restore.md`.

Deviations from §8:

| Plan | As built |
|---|---|
| Litestream off-site copies | Left out (untested, and it does not cover `files/`); rclone, a USB copy and a manual download are described instead |
| Every guide's commands run on a clean machine | Run: the README Docker quick start, the Compose file (`config`), the Caddy and nginx proxy settings (against a live server), the Linux `serve`/`setup-link` output, link and anchor check. **Not run:** Windows, Cloudflare Tunnel, Tailscale, certbot, rclone, cosign verification, systemd. The Windows, Cloudflare and Tailscale guides say so at the top |

Open points found while writing: `liturgist.exe` run by hand did not read `liturgist.env` (fixed in [§20](#20-the-windows-command-line-reads-liturgistenv-2026-10-05)); behind Docker a proxy on the same host appears as Docker's gateway address, so the guide tells the reader to trust that range, unconfirmed on a real setup; the System page's "not taken away" warning cannot see rclone copies.

## 19. Slice 6F as built (2026-10-05)

Merged to `main` as `edb4182`. Code: `server/https.go` (`builtInHTTPS`, `redirectToHTTPS`, `acmeTLS`), `Run` now starts a list of listeners (`server/server.go`), `parseDomain` in `internal/envconfig`, `healthURL(cfg)`, HTTPS mode `built_in` on the system page. Guide: [https-builtin.md](../self-host/https-builtin.md).

| Item | As built |
|---|---|
| Settings | `LITURGIST_DOMAIN` (turns it on), `LITURGIST_ACME_EMAIL`, `LITURGIST_HTTP_PORT` (80), `LITURGIST_HTTPS_PORT` (443), `LITURGIST_ACME_CA` (default Let's Encrypt production) |
| With a domain | `LITURGIST_LISTEN` is ignored (the Docker image sets it); `LITURGIST_BASE_URL` defaults to `https://<domain>` and must equal it if set; `LITURGIST_TRUSTED_PROXIES` is a configuration error; no start-up warnings |
| Listeners | HTTPS (app) on the HTTPS port; the plain port answers ACME HTTP-01, `/healthz`, `/readyz` and 308-redirects the rest. The TLS-ALPN challenge is answered on the HTTPS port. `AltHTTPPort`/`AltTLSALPNPort` are set to the configured ports so certmagic starts no listener of its own |
| Certificates | `<data>/certs` (certmagic file storage); not in backups (a test checks it) |
| Agreement | Changed in [§21](#21-liturgist_acme_agree-is-required-2026-10-05): the operator must set `LITURGIST_ACME_AGREE=true` |
| Health check | uses the plain port, no certificate needed |
| Dependency | `certmagic` v0.25.6 with 9 new modules (zerossl, cpuid, libdns, acmez, miekg/dns, blake3, zap, zap/exp) and `golang.org/x/net` v0.58 to v0.59; the binary grows about 1.8 MB (29.4 to 31.2 MB) |

Verified: a run against a Pebble test ACME server (certificate issued, HTTP/2 served, 308 redirect, `/healthz`, healthcheck, reuse after restart); Docker non-root binds 80 and 443 and is healthy; server tests with an injected self-signed certificate; seven mutations caught. **Not verified:** issuance from real Let's Encrypt, the systemd `AmbientCapabilities=CAP_NET_BIND_SERVICE` drop-in, the Windows service on ports 80 and 443. The certificate library's log now goes through the app log ([§22](#22-the-certificate-library-logs-through-the-app-log-2026-10-05)).

## 20. The Windows command line reads liturgist.env (2026-10-05)

A follow-up to 6D, which left commands typed by hand unaware of the service's settings. `loadServiceSettings` (`cmd/liturgist/envfile.go`) runs first in `run`: when `%ProgramData%\Liturgist\liturgist.env` exists it is read into the environment (variables already set win) and `LITURGIST_DATA_DIR` defaults to the service's `data` folder. Skipped for `service`, `version` and `openapi`; a malformed file stops the command and names the line; nothing happens when the file does not exist, and on Linux there is no service folder. `runService` uses the same `applySettings` and `defaultDataDir`; a settings file that exists but cannot be opened now stops the service with a logged error instead of being ignored. The guide `install-windows.md` says so, and warns not to run `serve` by hand beside the service. No line says which file was used (kept quiet so `backup` output stays clean).

Tests: TC-611 (read, environment wins, explicit data folder kept, no file, skipped commands, bad file), four mutations caught. **Not verified:** on a real Windows machine, including a folder that needs administrator rights to read.

## 21. LITURGIST_ACME_AGREE is required (2026-10-05)

Replaces the "agreement" row of §19: setting `LITURGIST_DOMAIN` no longer counts as accepting the certificate authority's subscriber agreement. With a domain, `LITURGIST_ACME_AGREE=true` (case-insensitive; any other value is refused) is required; without it `envconfig.Load` fails with the setting's name and the Let's Encrypt agreement URL (exit code 2). `Config.ACMEAgreed` carries it, and `acmeTLS` refuses to run without it, so a `Config` built in code cannot skip it. Without a domain the setting is not read. Chosen by the owner on 2026-10-05 because nothing was released yet, so the extra line breaks no one. The guides (`https-builtin.md`, `configuration.md`) show the line.

Tests: TC-612 (the setting is required, `""`, `false`, `yes` and `1` refused; the server refuses without it), the domain tests pass the agreement so each rejection has its own reason; three mutations caught. The certmagic flow itself was not rerun against Pebble; it sets `Agreed` as before, behind the check.

## 22. The certificate library logs through the app log (2026-10-05)

A follow-up to 6F. `internal/logging/zapbridge.go` is a `zapcore.Core` that writes the certificate library's zap lines to the slog logger, so they follow `LITURGIST_LOG_LEVEL` and `LITURGIST_LOG_FORMAT` and the redaction of the app; each carries `source=certmagic` (and `logger=<name>`). The library's `identifier` field is renamed `domain` because the redaction hides `identifier` as a login name. `go.uber.org/zap` becomes a direct dependency (it was already in the module graph).

**Fixes a 6F flaw:** `certmagic.NewDefault()` shares a process-wide cache whose renewal callback builds a fresh default config (default storage folder, no agreement), not the one 6F set up on the instance. `acmeTLS` now builds its own cache with a callback that returns its config, and stops the cache when the context ends. Found by reading the library; the old failure was not reproduced.

Tests: TC-613 (message, level, name and fields; level filter; redaction), four mutations caught (the fifth, the filter in `Check`, is redundant because zap checks the level first). Verified against a Pebble server: issuance with the bridged log, and a renewal through the cache using 150-second certificates and a temporary 5-second check interval (scratch build, not committed), with the data kept in `data/certs`. **Not verified:** real Let's Encrypt.
