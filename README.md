# LinkUp

**A URL shortener that does not watch the people who click.** Self-hosted, one
static binary, no telemetry.

[![CI](https://github.com/Ulzuhan/linkup/actions/workflows/ci.yml/badge.svg)](https://github.com/Ulzuhan/linkup/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.25%2B-00ADD8?logo=go&logoColor=white)](go.mod)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

Most shorteners are surveillance with a convenience feature attached: they log
addresses, fingerprint browsers, set cross-site cookies and sell what they
learn. LinkUp keeps the convenience and drops the rest.

## What "privacy-first" means here, precisely

Vague promises are worthless, so here is the exact shape of it:

- **A visitor's IP address is never read.** Not logged, not hashed, not counted.
  There is no `RemoteAddr` in the codebase.
- **The User-Agent is inspected in flight** when a link has device-specific
  destinations, and **never stored**. If you find it persisted anywhere, that is
  a bug and [a security report](SECURITY.md).
- **No cookies for visitors.** A cookie is only ever set for someone signing in
  to the dashboard.
- **Counters are aggregate**: total clicks and the timestamp of the last one.
  Nothing that can reconstruct who went where.
- **Tracking parameters are stripped** from destinations — and this is the part
  that takes care:

```
in   https://example.com/article?utm_source=newsletter&utm_medium=email&id=7
out  https://example.com/article?id=7
     stripped: utm_source, utm_medium
```

Removing query parameters is easy. Removing only the ones that track you, and
leaving the ones the page needs to work, is the whole job. `utm_*`, `fbclid`,
`gclid`, `msclkid`, `mc_eid` and `igshid` go; `id=7` stays.

**One consequence, stated plainly:** because no visitor is identifiable, there
is no per-visitor rate limit. Writes are budgeted per signed-in identity and PIN
attempts per link. A limiter that quietly looked at addresses would make the
promise above worthless, so there isn't one.

## Features

- **Custom or generated slugs** — `link.example.com/roadmap`, or a compact
  base56 slug.
- **Expiry by date or by click budget** — valid for 48 hours, or self-destruct
  after 50 visits.
- **PIN-protected links**, bcrypt-hashed, with an attempt budget per link.
- **Conditional routing** by device or locale, and A/B splitting.
- **Multiple domains** from one instance.
- **Destination preview** at `/preview/<slug>`: see where it goes, and what was
  stripped, before going.
- **Bulk import and export** as CSV.
- **Signed webhooks** (HMAC-SHA256) on link events.
- **REST API** with hashed API keys.
- **QR codes** via a [QR-Forge](https://github.com/Ulzuhan/qr-forge) instance.

## How fast, measured rather than claimed

```
goos: linux · goarch: amd64 · cpu: Intel(R) N150
BenchmarkLRUResolution-4    5390128    232.9 ns/op    320 B/op    1 allocs/op
```

A warm resolution costs about 230 ns and one allocation. Reproduce it with
`make bench`.

**And the honest footnote:** against a network round trip of 20–100 ms, that
number is noise. Nobody perceives it. Go was chosen for footprint — the
container is capped at 128 MB and never approaches it — and for a static binary
with almost nothing to scan for vulnerabilities. The speed is a pleasant
side effect, not the argument.
[ADR 0001](docs/decisions/0001-go-for-the-redirect-engine.md) sets that out in
full, including the reasons that do not hold.

## Architecture

```
   GET /roadmap
        │
        ▼
  ┌───────────┐  hit   ┌──────────────┐
  │ LRU cache │───────▶│ 301/302 out  │
  └─────┬─────┘        └──────────────┘
        │ miss
        ▼
  ┌──────────────┐     ┌──────────────┐
  │ SQLite (WAL) │────▶│ warm the LRU │
  └──────────────┘     └──────────────┘
```

- **Backend:** Go 1.25 with `chi`, no CGO.
- **Storage:** SQLite in WAL mode, pure-Go driver.
- **Dashboard:** server-rendered templates embedded in the binary with
  `embed.FS`. There is no separate frontend to deploy.
- **Auth:** OpenID Connect, with everything resolved from the provider's
  discovery document — no provider-specific URL is written anywhere in the code,
  so switching providers is changing one variable. Sessions are AES-GCM sealed
  cookies.

## Quick start

```bash
cp .env.example .env      # set LINKUP_SESSION_SECRET at minimum
make run                  # http://127.0.0.1:3464
```

For local work without an identity provider, set `LINKUP_DEV_MODE=true`. In that
mode every request without a cookie is treated as an administrator, so the
server **refuses to start unless bound to loopback**. That guard is deliberate.

With Docker:

```bash
docker compose up -d
```

See [DEPLOYMENT.md](DEPLOYMENT.md) for production: reverse proxy, OIDC provider
setup and the hardening the compose file already applies.

## Configuration

| Variable | What it does | Default |
|---|---|---|
| `LINKUP_PORT` | Port to listen on | `3464` |
| `LINKUP_HOST` | Bind address | `0.0.0.0` |
| `LINKUP_PUBLIC_HOST` | Public hostname, used for origin checks and canonical URLs | `localhost:3464` |
| `LINKUP_DEFAULT_DOMAIN` | Domain used for generated short links | value of `PUBLIC_HOST` |
| `LINKUP_SESSION_SECRET` | 32+ characters. Sessions reset on restart without it | *(ephemeral)* |
| `LINKUP_DB_PATH` | SQLite file | `./data/linkup.db` |
| `LINKUP_OIDC_ISSUER` | The issuer. The discovery document is derived from it | `""` |
| `LINKUP_OIDC_INTERNAL_BASE` | Optional: internal address for server-to-server calls to the provider | `""` |
| `LINKUP_OIDC_CLIENT_ID` / `_SECRET` | Client credentials | `""` |
| `LINKUP_OIDC_REDIRECT_URI` | Public callback URL | `http://localhost:3464/auth/callback` |
| `LINKUP_ENROLL_URL` | Where someone without an account is sent | `""` |
| `LINKUP_ACCOUNT_URL` | Where someone manages their account at the provider | `""` |
| `LINKUP_OIDC_PROVIDER_NAME` | What the sign-in button calls your provider | `your provider` |
| `LINKUP_ADMIN_GROUP` | Provider group whose members administer. Preferred | `""` |
| `LINKUP_ADMIN_USERS` | Username fallback. Ignored when a group is set | `""` |
| `LINKUP_ALLOW_PRIVATE_TARGETS` | Allow shortening to reserved ranges. Off by default | `false` |
| `LINKUP_QRFORGE_URL` | QR-Forge instance for QR generation | `""` |
| `LINKUP_DEV_MODE` | Local development only. See the warning above | `false` |

## Security

Reports go through a [private advisory](https://github.com/Ulzuhan/linkup/security/advisories/new);
the scope and the design constraints worth knowing before testing are in
[SECURITY.md](SECURITY.md).

Briefly: webhook destinations are validated when stored and again before every
delivery, on resolved addresses rather than hostnames, and the outbound client
does not follow redirects. API keys are stored as SHA-256 hashes. PINs use
bcrypt with a per-link attempt budget.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). In short: `make test`, `gofmt -l .` must
print nothing, commits follow Conventional Commits, and anything that would need
to identify a visitor needs a different design rather than an exception.

## License

MIT. See [LICENSE](LICENSE).
