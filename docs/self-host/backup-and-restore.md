# Backup and restore

> **Document type: Guide.** First draft with slice 6A of [14-self-host.md](../impl/14-self-host.md); the install guides and off-site details follow in slice 6E.

Everything Liturgist stores is in one **data folder** (`./data` by default, or `LITURGIST_DATA_DIR`): the database `liturgist.db`, the uploaded `files/` and the `backups/` folder. A backup is one `.zip` file with the database and `files/` inside. It holds every user's data, including password hashes: keep it as private as the database itself.

## Make a backup

```
liturgist backup                  # writes data/backups/manual-<date>.zip
liturgist backup /mnt/usb/church.zip
```

- It works while the server is running.
- It never overwrites a file; choose a new name.
- It checks the free space first and stops with a message if there is not enough.
- A copy in `data/backups/` is on the same disk as the database. **Also copy it somewhere else** (another computer, a USB stick, cloud storage): a disk failure takes both.

## Automatic backups

With SQLite, the server makes a backup every day at 02:00 (in the church's time zone; UTC before the church is set up) and writes it to `data/backups/auto-<UTC time>.zip`. It keeps the newest **7** of them, plus the newest one of each of the **4** weeks before those. Other files in `backups/` (manual backups, `pre-upgrade-*`, `pre-restore-*`) are never deleted by this. Old backups are deleted only after a new one succeeded, so a full disk never removes the last good copy.

| Setting | Default | Meaning |
|---|---|---|
| `LITURGIST_BACKUP_TIME` | `02:00` | Time of day, `HH:MM`, 24-hour; `off` turns automatic backups off |
| `LITURGIST_BACKUP_KEEP_DAILY` | `7` | How many of the newest backups to keep (1 to 365) |
| `LITURGIST_BACKUP_KEEP_WEEKLY` | `4` | How many older weeks keep one backup (0 to 365) |

- If the server was off at 02:00, it makes a backup 5 minutes after it starts when the newest automatic backup is more than 26 hours old.
- If the disk has too little free space, the backup is skipped and the server logs `backup_skipped`; other failures are logged as `backup_failed`. Neither stops the server.
- These backups are on the same disk as the database: **also copy them somewhere else**.
- A save that fails because the disk is full shows "Server storage is full. Ask the person who runs the server to free some space."

## Download a backup from the browser

People who can change church settings (the Church admin role) see **Settings → System**. It shows the version, database size, free disk space, the last backup, whether a copy was taken away, HTTPS and updates, and has a **Download backup** button. The download is the same zip as `liturgist backup`, made on the spot; it is logged (`backup_downloaded`, with who and how big, never the content). Treat the file like the database: it contains everyone's data and password hashes.

The page and a banner at the top of every page warn church admins when:

- the disk has under 1 GiB free, or under 5 % free if that is more;
- there has been no backup for 2 days;
- no backup was downloaded, or written outside the data folder with `liturgist backup /somewhere/else.zip`, for 30 days (a copy on the same disk does not survive the disk). A church younger than these limits is not warned yet;
- Liturgist is reachable over plain HTTP, or its address says https but no trusted proxy is set.

With PostgreSQL the download is not offered; use `pg_dump` (below).

### Update check (optional)

`LITURGIST_UPDATE_CHECK=true` makes the server ask `api.github.com` once a day for the newest release, and the System page then says whether an update is available. The request sends only the program name and version (`User-Agent: liturgist/<version>`), no data about your church. It is **off by default**, and nothing is ever installed automatically.

## Keep a copy somewhere else

Pick one. All three keep the newest nightly backup away from the server's disk.

- **By hand:** once a month, **Settings → System → Download backup** on your own computer.
- **A second disk or USB stick:** a nightly job (cron on Linux, Task Scheduler on Windows) that runs `liturgist backup /mnt/usb/liturgist-$(date +%F).zip`. A backup written outside the data folder counts as "taken away" for the System page warning.
- **Cloud storage with [rclone](https://rclone.org/):** after setting up a remote with `rclone config`, a nightly job such as

  ```
  rclone copy /var/lib/liturgist/backups remote:liturgist-backups --include "auto-*.zip"
  ```

  copies the automatic backups. The System page cannot see what rclone does, so it keeps warning after 30 days; the warning only reminds you, and the page is wrong in this case, not your copy. Check once in a while that the files are there.

Whichever you choose, do the [restore test](#test-your-backup-once) with a copy that came back from the other place.

## Restore a backup

1. **Stop the server** (`systemctl stop liturgist`, close the console window, or stop the container). `restore` refuses to run while the server is running.
2. Run `liturgist restore church.zip`. It checks the file (checksum, database integrity, version), shows when the backup was made, and asks you to type `yes`. In a script, add `--yes`.
3. Start the server again.

What `restore` does and keeps:

- The **current** database is kept as `data/backups/pre-restore-<time>.db` and the current `files/` folder as `data/backups/pre-restore-files-<time>/`. Delete them yourself once you are sure the restored data is right; Liturgist never deletes them.
- A backup made by an **older** Liturgist is fine: the next start upgrades it (after its own pre-upgrade copy).
- A backup made by a **newer** Liturgist is refused (exit code 3): upgrade the program first.
- If it stops half way, the message names the folder holding your previous data. Running `restore` again with the same file is safe.

### Test your backup once

A backup you have never restored is a hope. On a spare folder (the server need not stop, because the folder is different):

```
LITURGIST_DATA_DIR=/tmp/restore-test liturgist restore church.zip --yes
LITURGIST_DATA_DIR=/tmp/restore-test liturgist user list
```

The list should show your people. Delete `/tmp/restore-test` afterwards.

## Upgrades

Before applying a database migration, Liturgist copies the database to `data/backups/pre-upgrade-v<version>-….db` and keeps the newest 3. If an upgrade goes wrong, stop the server, install the old program, and copy the newest `pre-upgrade-*.db` over `data/liturgist.db`.

## PostgreSQL

`liturgist backup` and `restore` work with SQLite only. With PostgreSQL (`LITURGIST_DB_URL`), stop the server and run:

```
pg_dump --format=custom --file=liturgist.dump "$LITURGIST_DB_URL"
cp -a data/files files-backup          # the uploaded files are still in the data folder

pg_restore --clean --if-exists --no-owner --dbname="$LITURGIST_DB_URL" liturgist.dump
```

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Done |
| 1 | Failed: file exists, not enough space, damaged backup, server running, declined, no terminal and no `--yes` |
| 2 | Wrong command line or configuration |
| 3 | The backup was made by a newer Liturgist |
