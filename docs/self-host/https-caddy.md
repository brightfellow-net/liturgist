# HTTPS with Caddy

Caddy gets and renews the certificate by itself. You need a domain name (for example `liturgi.example.org`) pointing at the server, and ports 80 and 443 open to the internet.

## 1. Install Caddy

Follow [caddyserver.com/docs/install](https://caddyserver.com/docs/install) for your system.

## 2. Configure it

`/etc/caddy/Caddyfile`:

```
liturgi.example.org {
	reverse_proxy 127.0.0.1:8080
}
```

`sudo systemctl reload caddy`.

## 3. Tell Liturgist

Set these two (see [configuration.md](configuration.md)) and restart Liturgist:

```
LITURGIST_BASE_URL=https://liturgi.example.org
LITURGIST_TRUSTED_PROXIES=127.0.0.1
```

Caddy passes the visitor's address in `X-Forwarded-For`; trusting `127.0.0.1` lets Liturgist use it, so a login lock-out hits the person who mistyped and not everybody. With Docker, see the note on the gateway address in [install-docker.md](install-docker.md).

## 4. Check

Open `https://liturgi.example.org`. The address bar shows a padlock, and `journalctl -u liturgist` has no "plain HTTP" warning.
