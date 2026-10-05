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
