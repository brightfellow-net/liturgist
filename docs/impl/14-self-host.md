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
| H-2 | **`serve` holds a run lock** (`liturgist.run`, flock, non-blocking) for its whole life. A second `serve` on the same data folder refuses to start; `restore` refuses while it is held. This is how "server stopped" is checked, and it also stops two servers corrupting one folder | yes / no |
| H-3 | **Scheduler runs inside `serve`**, not cron. Daily at `LITURGIST_BACKUP_TIME` (default `02:00`, in the church's time zone, UTC before setup); `LITURGIST_BACKUP_TIME=off` turns it off. A missed run (server off at 02:00) runs 5 minutes after start when the newest automatic backup is over 26 hours old | in-process / cron docs only |
| H-4 | **Retention** `LITURGIST_BACKUP_KEEP_DAILY=7`, `LITURGIST_BACKUP_KEEP_WEEKLY=4`: keep the newest 7 automatic backups, plus the newest one of each of the 4 ISO weeks before them. Only `backups/auto-*.zip` is ever pruned; manual, `pre-upgrade-*` and `pre-restore-*` files are never touched by this | numbers |
| H-5 | **"Last backup" warning** on the system page and as a banner for church admins when the newest backup of any kind is over 48 hours old, **or** no backup was downloaded or written by `liturgist backup` to another path in 30 days (a copy on the same disk does not survive the disk) | thresholds |
| H-6 | **Disk-space warning** below 1 GiB free or 5% free, whichever is larger. A backup or pre-upgrade copy needs `1.2 ×` the database size plus the size of `files/` free, else it is skipped with a warning (a manual `backup` refuses with exit 1) | numbers |
| H-7 | **Built-in HTTPS (`certmagic`) is deferred** to slice 6F, after the pilot. It is a large new dependency and the pilot runs behind Caddy or a tunnel (guides in 6E). Needs your approval of the dependency when 6F starts | defer / build now |
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
| 4 | Extract to `<data dir>/restore-tmp/`; `PRAGMA integrity_check` and `PRAGMA foreign_key_check` on the extracted database; free space ≥ `1.2 ×` the current data | exit 1, tmp removed |
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

- **Last download** is recorded as the empty file `backups/.last-download` (its mtime); no migration. `last_at` is the newest mtime of `auto-*`, `manual-*` files.
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
| **6A** | `serve` run lock; `backup`; `restore`; the zip format; shared free-space helper; `backup-and-restore.md` (first draft) | H-1, H-2, H-6 |
| **6B** | Scheduler, retention, config; `storage_full` 507 and its strings | H-3, H-4 |
| **6C** | System routes, system page, banners, download | H-5, H-8, H-9, H-14 (+ H-13 if built) |
| **6D** | Dockerfile, GoReleaser, release workflow, systemd unit, Windows service, `healthcheck` | H-10, H-11, H-12 |
| **6E** | The rest of the guides and the README | none |
| **6F** | Built-in HTTPS with `certmagic` (deferred) | H-7 |

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
