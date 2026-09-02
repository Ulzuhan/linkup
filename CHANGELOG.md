# Changelog

Notable changes to LinkUp. Format based on [Keep a Changelog](https://keepachangelog.com/1.1.0/),
versioning follows [Semantic Versioning](https://semver.org/).

## [0.1.4] — 2026-09-02

### Fixed

- **Sign-in failed when the provider was reached over an internal address.**
  The provider derives the issuer from the Host it is asked on, so the token
  minted during the server-side code exchange carried the internal issuer while
  the verifier expected the public one. Both are the same provider and both are
  legitimate; both are now accepted, and every other issuer is still rejected —
  the check moved out of the library rather than being switched off.

## [0.1.3] — 2026-09-02

### Changed

- The sign-in button no longer names a specific identity product. It said
  "Login with Authentik", which is right for one deployment and wrong for every
  other; `LINKUP_OIDC_PROVIDER_NAME` decides, defaulting to "your provider".

## [0.1.2] — 2026-09-02

### Fixed

- **The site was served without styles.** Every `/static/*` request answered 404:
  `StaticFS` hangs off a sub-FS already rooted at `static`, so without
  `StripPrefix` the file server looked for `static/css/app.css` inside `static/`.
  The dashboard rendered unstyled, the health check passed and nothing failed
  loudly, which is how it reached production. Covered by a test that goes
  through the real router, because the bug was in how the handler was mounted.

### Security

- **Security headers now travel with the application**: Content-Security-Policy,
  Referrer-Policy, Permissions-Policy, and `X-Frame-Options: DENY`. A
  self-hosted copy gets the same protection as ours without knowing they exist.
  `X-XSS-Protection` is gone — obsolete, ignored, and harmful in the browsers
  that honoured it.

### Changed

- **Fonts are self-hosted.** The stylesheet's first line fetched them from
  Google on every page load, in a product whose argument is that it sends
  visitors nowhere. The three variable fonts ship inside the binary.
- **The QR preview no longer calls a third party.** It was fetched from
  `api.qrserver.com` with the short URL in the query string, handing away the
  one thing this product keeps. The button opens the operator's own QR-Forge.

## [0.1.1] — 2026-09-02

### Fixed

- The OIDC variable held the discovery document rather than the issuer, so the
  library appended `/.well-known/openid-configuration` to a URL that already
  ended in it and sign-in failed at the first step. Both variable names are
  accepted and the suffix is trimmed.
- Declares the Go version the module actually needs. `go.mod` asked for 1.27
  while the Dockerfile pinned 1.24; outside a container Go silently downloads
  the toolchain a module asks for, so CI never disagreed and the image build
  died the first time it ran.

### Added

- `LINKUP_OIDC_INTERNAL_BASE`, so server-to-server calls to the provider do not
  have to leave the host and come back.

## [0.1.0] — 2026-09-01

First release.

### Security

- **Webhook destinations are validated, and validated twice.** A target URL went
  from the database straight into an HTTP POST with no checks: an address in a
  reserved range was accepted and the server made that request. Destinations are
  now checked when stored and again before each delivery, on resolved addresses
  rather than on the hostname, and the outbound client no longer follows
  redirects — one hop undid the whole check.
- **The server refuses to start with an open panel.** With `LINKUP_DEV_MODE` on
  and OIDC unconfigured, any request without a cookie received an administrator
  session. That is now fatal at startup unless bound to loopback.
- **Abuse limits.** Writes are budgeted per authenticated identity and PIN
  attempts per link, with a growing lockout. Neither uses the visitor's address,
  because the product does not look at it.
- **Link destinations in reserved ranges are refused by default**, with
  `LINKUP_ALLOW_PRIVATE_TARGETS` for instances meant to shorten intranet URLs.
  Webhooks get no such switch.

### Added

- Administration resolved from an OIDC group (`LINKUP_ADMIN_GROUP`), with the
  username list kept only as a fallback for providers that emit no groups.
- Release workflow publishing to GHCR with SBOM and provenance, behind a Trivy
  gate that blocks fixable HIGH and CRITICAL findings.
- Weekly vulnerability scan of the published image.
- Renovate configuration.
- Security policy, contribution guide and the first architecture decision record.

### Fixed

- **The build.** `cmd/linkup/` was never committed: an unanchored `linkup`
  pattern in `.gitignore` matched the source directory as well as the compiled
  binary, and `go build ./...` passes without an entrypoint because it only
  compiles libraries. CI had been red since the first push.
- Module path now matches the repository URL, so `go get` works.
