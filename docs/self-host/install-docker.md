# Install with Docker

Needs Docker. The image runs as an unprivileged user and keeps everything in `/data`.

**No image is published yet.** Until the first release, build it from the source (below). The release will publish `ghcr.io/brightfellow-net/liturgist` (Linux amd64 and arm64); then replace `liturgist:local` with that name in the commands on this page.

## Build the image

```
git clone https://github.com/brightfellow-net/liturgist.git
cd liturgist
docker build -t liturgist:local .
```

## Try it

```
docker run -d --name liturgist \
  -p 127.0.0.1:8080:8080 \
  -v liturgist-data:/data \
  -e LITURGIST_BASE_URL=http://localhost:8080 \
  liturgist:local
docker logs liturgist
```

The log shows a framed **setup link**. Open it in your browser to create your church and the first administrator. A new link is printed by:

```
docker exec liturgist /liturgist setup-link
```

`-p 127.0.0.1:8080:8080` makes the app reachable from this computer only. To let others in, set up HTTPS first: [built-in HTTPS](https-builtin.md) (replaces this `docker run`), or put a proxy in front: [Caddy](https-caddy.md), [nginx](https-nginx.md), [Cloudflare Tunnel](https-cloudflare-tunnel.md) or [Tailscale](https-tailscale.md)); then set `LITURGIST_BASE_URL` to the public `https://` address and `LITURGIST_TRUSTED_PROXIES` to the proxy.

Seen from the container, a proxy running on the same computer comes from Docker's network gateway, not `127.0.0.1`. Look at the address in a request line of `docker logs` and trust that, or the whole range (`172.16.0.0/12` covers Docker's defaults).

## Compose

```yaml
services:
  liturgist:
    image: liturgist:local
    restart: unless-stopped
    ports: ["127.0.0.1:8080:8080"]
    volumes: ["liturgist-data:/data"]
    environment:
      LITURGIST_BASE_URL: https://liturgi.example.org
      LITURGIST_TRUSTED_PROXIES: 172.16.0.0/12
volumes:
  liturgist-data:
```

`docker compose up -d`. All variables are listed in [configuration.md](configuration.md).

## Commands

The program is `/liturgist` in the container:

```
docker exec liturgist /liturgist user list
docker exec liturgist /liturgist backup /data/backups/manual.zip
```

Automatic backups are written to `/data/backups` every night. **They live in the same volume as the data**, so also take a copy away: [backup-and-restore.md](backup-and-restore.md).

## Health

The image has a health check (`docker ps` shows `healthy`). It runs `/liturgist healthcheck`, which asks the server for `/healthz`.

## Upgrade

```
git pull
docker build -t liturgist:local .
docker rm -f liturgist     # the data stays in the volume
# run the same docker run command again
```

Read [upgrading.md](upgrading.md) first. Once the image is published, pull `ghcr.io/brightfellow-net/liturgist:latest` instead of building, and pin a version (`:0.1.0`) instead of `latest` if you want upgrades to be a decision.
