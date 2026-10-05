# Upgrading

## Before you upgrade

1. Check the release notes on the [releases page](https://github.com/brightfellow-net/liturgist/releases).
2. Take a backup and keep a copy elsewhere ([backup-and-restore.md](backup-and-restore.md)). Liturgist also copies the database by itself before changing it (`data/backups/pre-upgrade-v<version>-….db`, the newest 3 are kept), but a copy you made is the one you can count on. To make the upgrade refuse to go ahead without its own copy, set `LITURGIST_REQUIRE_PREUPGRADE_COPY=true`.

## Upgrade

| Installed as | Steps |
|---|---|
| Docker | `docker pull` the new image, remove the container, run it again ([install-docker.md](install-docker.md#upgrade)) |
| Linux | stop with `sudo systemctl stop liturgist`, `sudo install -m 0755 liturgist /usr/local/bin/liturgist`, then `sudo systemctl start liturgist` |
| Windows | `.\liturgist.exe service stop`, replace `liturgist.exe` in `C:\Program Files\Liturgist`, `.\liturgist.exe service start` |

The new program upgrades the database when it starts. Check the log for `database migrated`, then open the app. The System page (Settings → System) shows the version.

## Going back

A database that a newer version has upgraded cannot be used by an older one: the older program stops with exit code 3 and says so. **Going back means restoring a backup** taken before the upgrade:

1. Stop Liturgist and install the old program.
2. `liturgist restore <backup.zip>` ([backup-and-restore.md](backup-and-restore.md#restore-a-backup)).

Anything entered after that backup is lost. `--allow-newer-schema` starts an old program on a newer database anyway; it is for repair by someone who knows the data, not for going back.

## Verify a download

Each release has `checksums.txt` and `checksums.txt.sigstore.json`, a signature made by the project's release workflow on GitHub (no key to lose or steal). With [cosign](https://docs.sigstore.dev/cosign/system_config/installation/):

```
cosign verify-blob checksums.txt \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github.com/brightfellow-net/liturgist/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c --ignore-missing checksums.txt
```

The first command must say `Verified OK`; the second must say `OK` for your file. On Windows use `Get-FileHash <file>` and compare with the line in `checksums.txt`.

The Docker image is signed the same way:

```
cosign verify ghcr.io/brightfellow-net/liturgist:0.1.0 \
  --certificate-identity-regexp '^https://github.com/brightfellow-net/liturgist/\.github/workflows/release\.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```

Skipping the signature check is safe enough for a church server if you downloaded from the releases page itself and the checksum matches; the signature protects against a tampered download site.
