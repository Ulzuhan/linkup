# Deploying LinkUp

Everything below uses `example.com`. Substitute your own domains — and if you
copy a URL from here into a running instance without changing it, you will be
sending your users to somebody else's identity provider.

## What you are deploying

One container. A Go binary with the dashboard templates embedded in it, an
in-memory cache, and a SQLite file on a volume. No database server, no
frontend build, no sidecar.

```
internet ──▶ reverse proxy (TLS) ──▶ 127.0.0.1:3464 ──▶ linkup ──▶ /data/linkup.db
```

The container binds to loopback on purpose. Terminating TLS and reaching the
internet is the reverse proxy's job.

## 1. Identity provider

LinkUp authenticates the dashboard with OpenID Connect. Public redirects need no
sign-in; creating and managing links does.

Create a **confidential** client with the authorization code flow and set the
redirect URI to `https://link.example.com/auth/callback`.

**One trap worth naming, because it costs an afternoon.** In Authentik, a
provider created with an empty *Grant types* list rejects the request with
`invalid_request` **before showing a login page and without recording an
event** — so it looks like nothing happened at all. Make sure
`authorization_code` (and `refresh_token`, if you want sessions to renew) are
selected. To check from outside, ask for the authorize endpoint by hand: if it
redirects to a login flow you are fine, if it bounces straight back to your
callback with `error=`, that list is empty.

**Administration by group is strongly preferred.** Emit a `groups` claim and set
`LINKUP_ADMIN_GROUP`. Revoking an administrator then means removing them from a
group — no file to edit, no restart. `LINKUP_ADMIN_USERS` exists for providers
that emit no groups and is ignored whenever a group is configured, so the two
can never disagree.

## 2. Host directories

```bash
sudo install -d -m 0700 -o 10001 -g 10001 /srv/linkup/data
```

`10001` is the unprivileged user inside the image. The directory has to be
writable by it and by nothing else.

## 3. Configuration

```dotenv
LINKUP_PUBLIC_HOST=link.example.com
LINKUP_DEFAULT_DOMAIN=link.example.com
LINKUP_SESSION_SECRET=          # openssl rand -hex 32

LINKUP_OIDC_ISSUER=https://auth.example.com/application/o/linkup/
# Optional, and worth it when the provider is a container on the same host:
# LINKUP_OIDC_INTERNAL_BASE=http://authentik:9000
LINKUP_OIDC_CLIENT_ID=
LINKUP_OIDC_CLIENT_SECRET=
LINKUP_OIDC_REDIRECT_URI=https://link.example.com/auth/callback
LINKUP_ENROLL_URL=https://auth.example.com/if/flow/enroll-linkup/
LINKUP_ACCOUNT_URL=https://auth.example.com/if/user/

LINKUP_ADMIN_GROUP=linkup-admins
LINKUP_DB_PATH=/data/linkup.db
LINKUP_QRFORGE_URL=https://qr.example.com
```

Two things to get right:

- **Quote any value containing a space** if your setup loads this file with a
  shell rather than through Docker's own parser. `KEY=two words` unquoted makes
  a shell assign `KEY=two` and then try to run `words`, and the service dies in
  a restart loop with an error that never mentions the variable.
- **Never set `LINKUP_DEV_MODE=true` in production.** In that mode with OIDC
  unconfigured, every request without a cookie is treated as an administrator.
  The server refuses to start in that combination unless bound to loopback, but
  do not rely on the guard: do not set the variable.

## 4. Pin the image and start

```yaml
image: ghcr.io/ulzuhan/linkup:0.1.0@sha256:<digest>
```

Pin by digest, not by tag. A tag can be moved by whoever publishes it. The
release workflow prints the digest in its job summary.

```bash
docker compose up -d
docker compose ps                                                   # healthy
curl -s -o /dev/null -w '%{http_code}\n' http://127.0.0.1:3464/healthz   # 200
```

The compose file in this repository already applies `read_only`, drops all
capabilities, forbids privilege escalation, caps memory and PIDs and rotates
logs. Unrotated container logs are the most common way a small self-hosted box
runs out of disk.

## 5. Reverse proxy

### Caddy

```
link.example.com {
    encode gzip
    reverse_proxy 127.0.0.1:3464
}
```

### Nginx

```nginx
server {
    listen 443 ssl http2;
    server_name link.example.com;

    location / {
        proxy_pass http://127.0.0.1:3464;
        proxy_set_header Host $host;
    }
}
```

Do **not** add `X-Forwarded-For`. LinkUp does not read it, and forwarding a
visitor's address to a service that promises never to look at it only creates a
place for it to leak from.

## Backup and restore

SQLite is running in WAL mode, so copying the `.db` file while the service is
writing gives you a corrupt backup. Use the online backup API:

```bash
docker compose exec linkup sh -c 'sqlite3 /data/linkup.db ".backup /data/backup.db"' \
  2>/dev/null || docker compose stop linkup   # if sqlite3 is not in the image
cp /srv/linkup/data/backup.db /destination/linkup-$(date +%F).db
```

The image is minimal and does not ship `sqlite3`. The dependable route is to
stop the container for the seconds the copy takes — a redirect service can
afford that — or to snapshot the volume.

To restore: stop, replace `linkup.db` (deleting any `-wal` and `-shm` beside
it), start.

## Upgrading

Change the digest, then:

```bash
docker compose pull && docker compose up -d
```

Schema migrations run at startup, inside `database.Open`. To roll back, put the
previous digest back — but read what the newer version's migration did first: an
applied migration does not undo itself.
