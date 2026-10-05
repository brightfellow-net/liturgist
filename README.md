# Liturgist

Liturgist helps a church plan its services: the order of worship, songs and Scripture readings, a review step, and publishing the result to the people who serve. It is free software for a single church to run on its own computer, with an English and Indonesian interface (English first).

Status: before version 1.0. The pilot church is GKY Citragarden.

## Quick start

You need [Docker](https://docs.docker.com/get-docker/). In a terminal:

```
docker run -d --name liturgist -p 127.0.0.1:8080:8080 -v liturgist-data:/data \
  -e LITURGIST_BASE_URL=http://localhost:8080 ghcr.io/brightfellow-net/liturgist:latest
docker logs liturgist
```

Open the **setup link** shown in the log to create your church and its first administrator. That is a trial on one computer. For a real installation, with HTTPS and backups, follow a guide below.

## Install

| Guide | For |
|---|---|
| [Docker](docs/self-host/install-docker.md) | any system with Docker, the simplest |
| [Linux](docs/self-host/install-linux.md) | a Linux server or Raspberry Pi, with systemd |
| [Windows](docs/self-host/install-windows.md) | a Windows PC or server, as a service |

Then add HTTPS: [built in](docs/self-host/https-builtin.md) (Let's Encrypt, when the server has a public address), [Caddy](docs/self-host/https-caddy.md), [nginx](docs/self-host/https-nginx.md), [Cloudflare Tunnel](docs/self-host/https-cloudflare-tunnel.md) or [Tailscale](docs/self-host/https-tailscale.md).

## Look after it

- [Backup and restore](docs/self-host/backup-and-restore.md): nightly backups are automatic; keep a copy away from the server, and try a restore once.
- [Upgrading](docs/self-host/upgrading.md), including how to verify a download.
- [Configuration](docs/self-host/configuration.md): every setting.

## For developers

Go server (chi, Huma, sqlx; SQLite or PostgreSQL) and a React app (Vite, TanStack Query, Tailwind). The product is described in [docs/SPEC.md](docs/SPEC.md) and the design in [docs/impl/](docs/impl/README.md).

```
make build      # generates the API client, builds the web app, then bin/liturgist
make test       # Go and web unit tests
make e2e        # browser tests
```

Licence: Apache-2.0 ([LICENSE](LICENSE)).
