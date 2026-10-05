# HTTPS with a Cloudflare Tunnel

Use this when the server is at home or behind a router and you cannot open ports. It needs a free Cloudflare account and a domain managed there. Visitors' traffic passes through Cloudflare.

> **Not tested by the project.** Cloudflare's screens change; the settings Liturgist needs are in step 3.

## 1. Create the tunnel

In the Cloudflare dashboard, open **Zero Trust → Networks → Tunnels**, create a tunnel, and install `cloudflared` on the Liturgist computer with the command it shows.

## 2. Point a name at Liturgist

Add a **public hostname**: `liturgi.example.org`, service type **HTTP**, URL `127.0.0.1:8080`.

## 3. Tell Liturgist

```
LITURGIST_BASE_URL=https://liturgi.example.org
LITURGIST_TRUSTED_PROXIES=127.0.0.1
LITURGIST_CLIENT_IP_HEADER=CF-Connecting-IP
```

`cloudflared` connects from the same computer, so `127.0.0.1` is the proxy; Cloudflare puts the visitor's address in `CF-Connecting-IP`. Restart Liturgist ([configuration.md](configuration.md)).

## 4. Check

Open `https://liturgi.example.org`. Do not also expose port 8080 to the internet, or visitors could skip Cloudflare.
