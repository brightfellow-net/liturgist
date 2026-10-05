# HTTPS with nginx and Let's Encrypt

For a server that already runs nginx. You need a domain name pointing at the server and ports 80 and 443 open.

## 1. Site

`/etc/nginx/sites-available/liturgist`, linked into `sites-enabled`:

```
server {
	listen 80;
	server_name liturgi.example.org;

	client_max_body_size 20m;

	location / {
		proxy_pass http://127.0.0.1:8080;
		proxy_set_header Host $host;
		proxy_set_header X-Forwarded-For $remote_addr;
		proxy_set_header X-Forwarded-Proto $scheme;
		proxy_read_timeout 120s;
	}
}
```

`$remote_addr` (not `$proxy_add_x_forwarded_for`) means a visitor cannot send a made-up address through the proxy.

## 2. Certificate

```
sudo apt install certbot python3-certbot-nginx
sudo certbot --nginx -d liturgi.example.org
```

Certbot edits the site to use HTTPS and renews the certificate on a timer.

## 3. Tell Liturgist

```
LITURGIST_BASE_URL=https://liturgi.example.org
LITURGIST_TRUSTED_PROXIES=127.0.0.1
```

Restart Liturgist and open the address. See [configuration.md](configuration.md).
