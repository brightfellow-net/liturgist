# Install on Windows

For Windows 10 or 11 and Windows Server 2019 or newer, 64-bit. The program runs as a Windows service, so it starts with the computer.

> **Not yet tested on a real Windows machine.** Report anything that does not work.

## 1. Download

Download `liturgist_<version>_windows_amd64.zip` from the [releases page](https://github.com/brightfellow-net/liturgist/releases) and unzip it to `C:\Program Files\Liturgist`. To check the file, see [upgrading.md](upgrading.md#verify-a-download).

## 2. Install the service

Open **PowerShell as Administrator**:

```
cd "C:\Program Files\Liturgist"
.\liturgist.exe service install
```

This creates `C:\ProgramData\Liturgist` with the settings file `liturgist.env`. The data goes to `C:\ProgramData\Liturgist\data` and the log to `C:\ProgramData\Liturgist\liturgist.log`. The service restarts itself if it stops unexpectedly.

## 3. Settings (optional)

Edit `C:\ProgramData\Liturgist\liturgist.env` in Notepad, one `NAME=value` per line, for example:

```
LITURGIST_BASE_URL=https://liturgi.example.org
LITURGIST_TRUSTED_PROXIES=127.0.0.1
```

All settings: [configuration.md](configuration.md). Lines starting with `#` are ignored.

## 4. Start

```
.\liturgist.exe service start
```

Open `C:\ProgramData\Liturgist\liturgist.log` and find the framed **setup link**; open it in your browser to create your church and first administrator.

## Commands by hand

Commands such as `user list` or `backup` do not read `liturgist.env`. Tell them where the data is first:

```
$env:LITURGIST_DATA_DIR = "C:\ProgramData\Liturgist\data"
$env:LITURGIST_BASE_URL = "https://liturgi.example.org"
.\liturgist.exe setup-link
```

## Day to day

```
.\liturgist.exe service stop
.\liturgist.exe service start     # after changing liturgist.env
.\liturgist.exe service uninstall # keeps your data
```

`liturgist.log` is not trimmed automatically; delete or archive it now and then while the service is stopped.

## HTTPS and backups

The server listens on `127.0.0.1:8080` only. For HTTPS use [Caddy](https-caddy.md) (it also runs on Windows), [Cloudflare Tunnel](https-cloudflare-tunnel.md) or [Tailscale](https-tailscale.md). Nightly backups go to `C:\ProgramData\Liturgist\data\backups`; take a copy elsewhere: [backup-and-restore.md](backup-and-restore.md).
