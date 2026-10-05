# HTTPS with Tailscale

Use this when only your own people, each with Tailscale installed, need to reach Liturgist. Nothing is open to the public internet, and no domain name is needed. Tailscale supplies an `https://<machine>.<tailnet>.ts.net` address with a real certificate.

> **Not tested by the project.** Check the commands against Tailscale's current documentation.

## 1. Prepare

Install Tailscale on the server and on each person's phone or computer, sign in to the same tailnet, and enable **HTTPS Certificates** in the Tailscale admin console (DNS page).

## 2. Publish Liturgist inside the tailnet

```
sudo tailscale serve --bg 8080
tailscale serve status
```

`status` shows the address, for example `https://server.tail1234.ts.net`.

## 3. Tell Liturgist

```
LITURGIST_BASE_URL=https://server.tail1234.ts.net
LITURGIST_TRUSTED_PROXIES=127.0.0.1
```

Restart Liturgist ([configuration.md](configuration.md)) and open the address from a device on the tailnet. Do not use `tailscale funnel` unless you mean to open Liturgist to the whole internet.
