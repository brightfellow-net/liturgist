# Configuration

Liturgist is configured with environment variables. Every one is optional; the defaults suit a trial on one computer. A wrong value stops the server at start with a message naming the variable.

| Variable | Default | Meaning |
|---|---|---|
| `LITURGIST_DATA_DIR` | `./data` | Folder for the database, uploaded files and backups. Back this up (see [backup-and-restore.md](backup-and-restore.md)). |
| `LITURGIST_LISTEN` | `127.0.0.1:8080` | Address and port to listen on. `:8080` listens on every network interface. |
| `LITURGIST_BASE_URL` | `http://localhost:8080` | The address people type in the browser, with no path. Used in links (invitations, setup) and to decide whether cookies are `Secure`. Use the `https://` address once you have HTTPS. |
| `LITURGIST_EXTRA_HOSTS` | empty | Other host names or IPs the server answers to, for example `192.168.1.10,gereja.local`. Any other `Host` gets a 421 error. |
| `LITURGIST_TRUSTED_PROXIES` | empty | Addresses or ranges of the reverse proxy or tunnel in front of Liturgist, for example `127.0.0.1`. Only these may tell Liturgist a visitor's real IP address. Leave empty without a proxy. |
| `LITURGIST_CLIENT_IP_HEADER` | empty | The header carrying the visitor's IP from a trusted proxy, for example `CF-Connecting-IP`. Empty means `X-Forwarded-For`. |
| `LITURGIST_BACKUP_TIME` | `02:00` | Time of the daily automatic backup, in the church's time zone, or `off`. |
| `LITURGIST_BACKUP_KEEP_DAILY` | `7` | Newest automatic backups kept (1 to 365). |
| `LITURGIST_BACKUP_KEEP_WEEKLY` | `4` | Further weeks that keep one backup each (0 to 365; 0 keeps none). |
| `LITURGIST_UPDATE_CHECK` | `false` | `true` asks GitHub once a day whether a newer version exists and shows it on the System page. Nothing else is sent. |
| `LITURGIST_AUTO_MIGRATE` | `true` | Upgrade the database when a newer program starts. |
| `LITURGIST_REQUIRE_PREUPGRADE_COPY` | `false` | `true` refuses to upgrade the database if the copy made before the upgrade fails. |
| `LITURGIST_SESSION_TTL` | `2160h` | How long a login lasts without use (90 days). At least `1h`. |
| `LITURGIST_SESSION_MAX_AGE` | `8760h` | The longest a login lasts, used or not (one year). |
| `LITURGIST_LOG_FORMAT` | `text` | `text` or `json`. |
| `LITURGIST_LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error`. |
| `LITURGIST_DB_DRIVER` | `sqlite` | `sqlite` or `postgres`. SQLite is right for one church. |
| `LITURGIST_DB_URL` | empty | PostgreSQL connection URL; required with `postgres`. |

## Where to put them

| Installed as | Put the variables in |
|---|---|
| Docker | `-e NAME=value` on `docker run`, or `environment:` in Compose ([install-docker.md](install-docker.md)) |
| Linux with systemd | `/etc/liturgist/liturgist.env`, one `NAME=value` per line ([install-linux.md](install-linux.md)) |
| Windows service | `C:\ProgramData\Liturgist\liturgist.env`, same format ([install-windows.md](install-windows.md)) |

Change a variable, then restart the server.

## Warnings at start

Liturgist logs a warning, and carries on, when:

- it listens on the network, `LITURGIST_BASE_URL` starts with `http://` and no proxy is trusted: passwords can be read on the network. Put HTTPS in front of it ([Caddy](https-caddy.md), [nginx](https-nginx.md), [Cloudflare Tunnel](https-cloudflare-tunnel.md), [Tailscale](https-tailscale.md)).
- `LITURGIST_BASE_URL` starts with `https://` but `LITURGIST_TRUSTED_PROXIES` is empty.
