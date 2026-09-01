# Changelog

Notable changes to LinkUp. Format based on [Keep a Changelog](https://keepachangelog.com/1.1.0/),
versioning follows [Semantic Versioning](https://semver.org/).

## [Unreleased]

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
