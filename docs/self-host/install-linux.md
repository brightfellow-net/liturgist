# Install on Linux (systemd)

For a Linux server or a Raspberry Pi 4 or newer (64-bit). You need `sudo`.

## 1. Download and check

Download `liturgist_<version>_linux_amd64.tar.gz` (or `_arm64` for a Raspberry Pi) and `checksums.txt` from the [releases page](https://github.com/brightfellow-net/liturgist/releases), then:

```
sha256sum -c --ignore-missing checksums.txt
```

It must say `OK`. To also prove that the project's release process made the file, see [upgrading.md](upgrading.md#verify-a-download).

## 2. Install

```
tar xzf liturgist_*_linux_*.tar.gz
sudo useradd --system --home /var/lib/liturgist --shell /usr/sbin/nologin liturgist
sudo install -m 0755 liturgist /usr/local/bin/liturgist
sudo install -m 0644 liturgist.service /etc/systemd/system/
sudo install -d /etc/liturgist
```

## 3. Settings

Create `/etc/liturgist/liturgist.env` (all optional, see [configuration.md](configuration.md)):

```
LITURGIST_BASE_URL=https://liturgi.example.org
LITURGIST_TRUSTED_PROXIES=127.0.0.1
```

Without HTTPS yet, leave the file out and use the app from the same computer only (`http://localhost:8080`).

## 4. Start

```
sudo systemctl daemon-reload
sudo systemctl enable --now liturgist
sudo journalctl -u liturgist -n 20
```

The log shows a framed **setup link**; open it to create your church and first administrator. A new link:

```
sudo -u liturgist env LITURGIST_DATA_DIR=/var/lib/liturgist LITURGIST_BASE_URL=https://liturgi.example.org liturgist setup-link
```

The same `sudo -u liturgist env LITURGIST_DATA_DIR=/var/lib/liturgist …` prefix is needed for every command you run by hand (`user list`, `backup`, `restore`), so the data is found and its files keep the right owner.

## 5. HTTPS

The server listens on `127.0.0.1:8080` only. Either let it serve HTTPS itself ([built-in HTTPS](https-builtin.md)), or put one of these in front: [Caddy](https-caddy.md) (simplest), [nginx](https-nginx.md), [Cloudflare Tunnel](https-cloudflare-tunnel.md) or [Tailscale](https-tailscale.md).

## 6. Backups

Nightly backups are automatic, in `/var/lib/liturgist/backups`. Take a copy off the computer: [backup-and-restore.md](backup-and-restore.md).

## Day to day

```
sudo systemctl status liturgist
sudo systemctl restart liturgist     # after changing liturgist.env
sudo journalctl -u liturgist -f
```
