# Built-in HTTPS (Let's Encrypt)

Liturgist can get its own free certificate from [Let's Encrypt](https://letsencrypt.org/) and serve HTTPS itself, with no Caddy, nginx or tunnel. Use this when the server is on the internet with its own address. If it is behind a router at home, use a [Cloudflare Tunnel](https-cloudflare-tunnel.md) or [Tailscale](https-tailscale.md) instead; with a reverse proxy already in place use [Caddy](https-caddy.md) or [nginx](https-nginx.md).

## You need

- A domain name, for example `liturgi.example.org`, whose DNS record points at the server's public address.
- Ports **80 and 443** open to the internet and used by nothing else on the machine. Let's Encrypt connects to them to check that the name is yours.
- A computer clock that is right.

## Turn it on

Set one variable (see [configuration.md](configuration.md)) and start Liturgist:

```
LITURGIST_DOMAIN=liturgi.example.org
LITURGIST_ACME_AGREE=true
LITURGIST_ACME_EMAIL=you@example.org
```

`LITURGIST_ACME_EMAIL` is optional; Let's Encrypt uses it only to warn you if a certificate is about to expire without renewing. **`LITURGIST_ACME_AGREE=true` says that you have read and accept the [Let's Encrypt subscriber agreement](https://letsencrypt.org/repository/)** (or that of the other certificate authority you chose); with a domain, Liturgist does not start without it. `LITURGIST_BASE_URL` becomes `https://liturgi.example.org` by itself; if you set it, it must be exactly that. `LITURGIST_LISTEN` is ignored, and `LITURGIST_TRUSTED_PROXIES` must stay empty.

The first start takes up to a minute while the certificate is fetched; the browser shows an error until it is ready. The log says what happens (lines starting with a number are from the certificate library). Certificates renew by themselves and are kept in `data/certs`. They are **not** in backups, because they can be fetched again.

Port 80 only redirects to HTTPS (and answers Let's Encrypt and the health check).

| Installed as | Add |
|---|---|
| Docker | `-p 80:80 -p 443:443 -e LITURGIST_DOMAIN=… -e LITURGIST_ACME_AGREE=true` instead of `-p 8080:8080`; keep the `/data` volume so the certificate survives |
| Linux (systemd) | the settings in `/etc/liturgist/liturgist.env`, and permission to use ports below 1024: `sudo systemctl edit liturgist` and add `[Service]` / `AmbientCapabilities=CAP_NET_BIND_SERVICE` |
| Windows | the settings in `liturgist.env`; allow the firewall prompt for ports 80 and 443 |

## Try it with the staging server first

Let's Encrypt limits how often a real certificate may be requested, so a mistake in DNS or the firewall can lock you out for a while. To test, add

```
LITURGIST_ACME_CA=https://acme-staging-v02.api.letsencrypt.org/directory
```

The browser will warn that the staging certificate is untrusted; that is expected. When the log shows the certificate was obtained, remove the line, stop Liturgist, delete `data/certs`, and start it again.

## If it does not work

| Symptom | Likely cause |
|---|---|
| The log repeats "could not get certificate" | DNS does not point here yet, or ports 80 or 443 are closed or used by another program |
| "permission denied" for port 80 or 443 | the program is not allowed to use ports below 1024 (see the table above) |
| "address already in use" | another web server runs on 80 or 443; stop it, or use a proxy guide |
| Works from outside but not inside the office | the router does not send the office's own requests to the public address; give the office computers the server's local address for the name |

## Other ports

`LITURGIST_HTTP_PORT` and `LITURGIST_HTTPS_PORT` (default 80 and 443) change the ports Liturgist listens on, for when a router or Docker maps public 80 and 443 to them (for example `-p 80:8080 -p 443:8443` with `LITURGIST_HTTP_PORT=8080` and `LITURGIST_HTTPS_PORT=8443`). Let's Encrypt always connects to public 80 and 443.
