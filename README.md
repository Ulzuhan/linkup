# LinkUp

**Privacy-first URL shortener, clean redirect engine & tracking stripper.** Self-hosted, lightweight, and sovereign.

[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.24+-00ADD8.svg)](go.mod)
[![Architecture](https://img.shields.io/badge/Architecture-In--Memory%20LRU%20%2B%20SQLite%20WAL-indigo.svg)](README.md)

Most URL shorteners act as mass surveillance networks: they fingerprint visitors, record IP addresses, plant cross-site tracking cookies, and monetize user browsing habits. **LinkUp** is an ultra-fast, privacy-respecting link redirect engine designed to strip invasive tracking parameters, offer custom branded slugs, provide link expiration, and collect only strictly aggregated, non-identifying access counts.

---

## 🔒 Privacy & Data Doctrine

- **Zero Visitor Profiling**: No IP logging, no User-Agent fingerprinting, no tracking cookies, and no referrer profiling.
- **Tracking Parameter Stripper**: Automatically detects and strips invasive URL query parameters on creation or redirect (`utm_*`, `fbclid`, `gclid`, `msclkid`, `mc_eid`, `igshid`, etc.).
- **Aggregated Metrics Only**: Counters store only total lifetime clicks and the timestamp of the last redirection.
- **Zero Third-Party Leaks**: Zero telemetry beacons or external scripts.

---

## ⚡ Core Features

- **Custom Slugs / Aliases**: Support for human-readable custom aliases (e.g. `link.kaicorplabs.com/roadmap`) and random compact base56 alphanumeric slugs.
- **Sub-microsecond Redirection Engine**: In-memory concurrent LRU cache serving warm redirects in nanoseconds.
- **Link Expiration & Access Limits**:
  - Expiry by date/time (e.g., valid for 48 hours).
  - Expiry by maximum click budget (e.g., self-destruct after 50 visits).
- **Password / PIN Gate**: Optional PIN protection for confidential destinations.
- **Deep Integration with QR-Forge**: Instant generation of vector/raster QR codes for any shortened link without manual copy-pasting.
- **Instant Destination Preview**: Safety inspection screen (`/preview/<slug>`) allowing users to inspect the destination and stripped parameters before redirecting.

---

## 👥 Access & Permissions Model

- **Redirection / Public Consumption**: Completely public and blazing fast (`301` or `302` direct redirects).
- **Link Management & Creation**: Protected by OIDC (e.g. Authentik). Users manage their own links, view aggregated stats, update destination targets, or delete links.
- **Admin / Quarantine Capabilities**: Ability to disable or purge abusive destination domains.

---

## 🛠️ Architecture & Tech Stack

- **Backend**: Go (Go 1.24+ / `chi`) with in-memory LRU cache (< 100ns per cached resolution).
- **Frontend**: Lightweight dashboard embedded into the binary with `embed.FS` matching the KaiCorp theme (*Space Grotesk*, *Inter*, *JetBrains Mono*).
- **Persistence**: SQLite (with WAL mode enabled for rapid concurrent reads, pure Go driver without CGO requirements).
- **Auth**: OpenID Connect (OIDC / Authentik) with AES-GCM encrypted session cookies.
- **Container / Isolation**: `read_only: true`, `cap_drop: [ALL]`, non-root user (`uid 10001`), loopback port binding (`127.0.0.1:3464`).

---

## 🚀 Quick Start

### Local Development

```bash
# Copy example environment
cp .env.example .env

# Run local development server
make run
```

Access the dashboard at `http://localhost:3464`.

### Run Tests & Benchmarks

```bash
# Run unit and integration tests with race detector
make test

# Run microsecond resolution benchmarks
make bench
```

### Build Single Autonomous Binary

```bash
make build
# Produces ./bin/linkup (~15MB single standalone executable)
```

---

## ⚙️ Environment Variables

| Variable | Description | Default |
|---|---|---|
| `LINKUP_PORT` | Port to listen on. | `3464` |
| `LINKUP_HOST` | Host address to bind to. | `0.0.0.0` |
| `LINKUP_PUBLIC_HOST` | Explicit public hostname for origin guarding. | `localhost:3464` |
| `LINKUP_DEFAULT_DOMAIN` | Base domain for shortened links. | `link.kaicorplabs.com` |
| `LINKUP_SESSION_SECRET` | 32+ char secret key for encrypting sessions. | *(auto-generated in dev)* |
| `LINKUP_OIDC_CLIENT_ID` | OIDC Client ID. | `""` |
| `LINKUP_OIDC_CLIENT_SECRET` | OIDC Client Secret. | `""` |
| `LINKUP_OIDC_DISCOVERY_URL` | OIDC Issuer / Discovery URL (`/.well-known/openid-configuration`). | `""` |
| `LINKUP_OIDC_REDIRECT_URI` | Public callback URL (e.g. `https://link.kaicorplabs.com/auth/callback`). | `http://localhost:3464/auth/callback` |
| `LINKUP_ENROLL_URL` | Self-service account registration URL. | `""` |
| `LINKUP_ACCOUNT_URL` | Provider user profile / account management URL. | `""` |
| `LINKUP_DB_PATH` | Path to SQLite database. | `./data/linkup.db` |
| `LINKUP_QRFORGE_URL` | Base URL of QR-Forge instance for seamless QR integration. | `https://qr.kaicorplabs.com` |
| `LINKUP_ADMIN_USERS` | Comma-separated list of admin usernames. | `""` |
| `LINKUP_DEV_MODE` | Set `true` to enable local testing without an active OIDC provider. | `false` |

---

## 🐳 Docker Deployment

See [DEPLOYMENT.md](DEPLOYMENT.md) for full production deployment instructions with Authentik, Docker Compose, and Caddy/Nginx.

```bash
docker compose up -d --build
```
