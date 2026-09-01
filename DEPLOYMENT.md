# LinkUp Deployment & Operations Guide

This guide details how to deploy, configure, and maintain **LinkUp** within the KaiCorp Labs sovereign infrastructure.

---

## 🏗️ Architecture Overview

* **Engine**: Go 1.24+ compiled to an isolated static binary with embedded templates & styles (`embed.FS`).
* **Storage**: SQLite 3 with Write-Ahead Logging (`WAL`) mode enabled.
* **Cache**: In-memory LRU cache (< 100ns resolution latency for warm slugs).
* **Isolation**: Non-root container (`UID 10001`), `cap_drop: [ALL]`, bounded to loopback `127.0.0.1:3464`.
* **Auth**: OIDC via Authentik with encrypted AES-GCM session cookies.

---

## 🔐 Authentik OIDC Configuration

1. **Create Provider in Authentik**:
   * **Type**: `OAuth2/OpenID Provider`.
   * **Name**: `LinkUp Provider`.
   * **Client Type**: `Confidential`.
   * **Client ID**: (e.g. `linkup-client-id`).
   * **Client Secret**: (Generate a secure string).
   * **Redirect URIs**: `https://link.kaicorplabs.com/auth/callback`.
   * **Signing Key**: Select Authentik default self-signed cert.
   * **Scopes**: `openid`, `profile`, `email`.

2. **Create Application in Authentik**:
   * **Name**: `LinkUp`.
   * **Slug**: `linkup`.
   * **Provider**: Select `LinkUp Provider`.
   * **Launch URL**: `https://link.kaicorplabs.com`.

---

## 🚀 Production Deployment

### 1. Directory Structure on Host

```bash
sudo mkdir -p /srv/kaicorp/linkup/data
sudo chown -R 10001:10001 /srv/kaicorp/linkup/data
sudo chmod 750 /srv/kaicorp/linkup/data
```

### 2. Environment File (`.env`)

Create `/srv/kaicorp/linkup/.env`:

```ini
LINKUP_PORT=3464
LINKUP_HOST=0.0.0.0
LINKUP_PUBLIC_HOST=link.kaicorplabs.com
LINKUP_DEFAULT_DOMAIN=link.kaicorplabs.com

# 32+ char secret key
LINKUP_SESSION_SECRET=replace_with_a_secure_random_hex_string_min_32_chars

# Authentik OIDC Settings
LINKUP_OIDC_CLIENT_ID=linkup-client-id
LINKUP_OIDC_CLIENT_SECRET=linkup-client-secret
LINKUP_OIDC_DISCOVERY_URL=https://auth.kaicorplabs.com/application/o/linkup/.well-known/openid-configuration
LINKUP_OIDC_REDIRECT_URI=https://link.kaicorplabs.com/auth/callback
LINKUP_ENROLL_URL=https://auth.kaicorplabs.com/if/flow/default-enrollment-flow/
LINKUP_ACCOUNT_URL=https://auth.kaicorplabs.com/if/user/#/settings

# Persistence & Integrations
LINKUP_DB_PATH=/data/linkup.db
LINKUP_QRFORGE_URL=https://qr.kaicorplabs.com
LINKUP_ADMIN_USERS=admin,ulzuhan
```

### 3. Start Container

```bash
docker compose -f compose.yaml up -d --build
```

---

## 🌐 Reverse Proxy Configuration

### Caddyfile

```caddy
link.kaicorplabs.com {
    reverse_proxy 127.0.0.1:3464 {
        header_up Host {host}
        header_up X-Real-IP {remote_host}
        header_up X-Forwarded-Proto {scheme}
    }
}
```

### Nginx

```nginx
server {
    server_name link.kaicorplabs.com;
    listen 443 ssl http2;

    location / {
        proxy_pass http://127.0.0.1:3464;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

---

## 💾 Backup & Restore

Since SQLite is operating in WAL mode, create atomic hot backups using `sqlite3`:

```bash
# Backup live database without stopping the container
sqlite3 /srv/kaicorp/linkup/data/linkup.db ".backup '/srv/kaicorp/linkup/backups/linkup_$(date +%Y%m%d_%H%M%S).db'"
```
