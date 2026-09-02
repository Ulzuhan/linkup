# Changelog

Notable changes to LinkUp. Format based on [Keep a Changelog](https://keepachangelog.com/1.1.0/),
versioning follows [Semantic Versioning](https://semver.org/).

## [0.3.0] — 2026-09-02

### Added

- **Links can be edited.** An Edit button on every link opens a dialog with
  everything but the address: destination (cleaned again on save), title,
  folder, tags, redirect code, PIN — set a new one or remove it —, expiry, click
  budget, whether the link is active, and the per-device targets. Moving a
  link between folders is the folder field of that dialog.
- **Folders can be renamed and deleted.** With a folder selected, Rename and
  Delete folder appear next to the tabs. Deleting a folder never deletes a
  link: its links go back to All links, in the same transaction, and the
  confirmation says so.
- `PATCH /api/folders/{id}` renames or recolours a folder.

### Changed

- `PATCH /api/links/{id}` now takes `expires_in_hours` and `redirect_type`,
  treats zero as "clear" for the click budget and the expiry, cleans the iOS
  and Android targets like the main one, and refuses a folder that is not
  yours — a folder id is not a secret, and a link moved into somebody else's
  folder would appear in their view.

## [0.2.3] — 2026-09-02

### Changed

- **The footer is exactly the house's.** The tagline under the common line
  ("Sovereign, privacy-first redirect infrastructure…") is gone: it read well
  on its own and broke the one thing the footer is for, which is looking the
  same in every tool. The promise it carried lives on the front page.

## [0.2.2] — 2026-09-02

### Changed

- **The Content-Security-Policy no longer allows inline styles.** The
  templates carried 163 `style=` attributes and the policy had to say
  `'unsafe-inline'` for them; every one moved into the stylesheet, the one
  piece of data that used to travel in a style attribute — a folder's colour —
  is an SVG fill now, and the box the script used to show and hide goes by the
  `hidden` attribute. A test renders every page with a link that has a PIN,
  tags and a folder and fails on the first inline style that comes back.
- **The public pages — where a link goes, the PIN, the errors — are composed
  like the rest**: same workspace, same card, no emoji medallions.
- The README shows the dashboard.

## [0.2.1] — 2026-09-02

### Fixed

- **The site could look unstyled for hours after a deploy.** The stylesheet
  changed behind the same address and nothing here said how long it could be
  kept, so the CDN in front served the previous version for its default four
  hours while the new pages asked for classes it did not have. Assets now carry
  the build's digest in their URL — a new build is a new address — and answer
  with cache headers to match: versioned files are immutable, anything else
  lives a day.

## [0.2.0] — 2026-09-02

### Added

- **A front page.** An anonymous visitor used to land on the dashboard with
  nothing in it and a sign-in button in the corner. The root is now a front
  page: what the tool does, a link before and after, where to sign in and where
  to ask for an account. Signed in, the same address is the workspace.

### Changed

- **The chrome is the house's.** Header and footer are the same bar and the
  same foot as in the other KaiCorp Labs tools: the mark, the tool's name, and
  on the right one account menu — who you are, settings, your account at the
  provider, sign out. The foot says who built it and, only when the operator
  sets `KAICORP_FOOTER_LINKS`, links to the other tools: in somebody else's
  deployment those links are advertising, so by default they are not there.
  The tagline stays in the foot.
- **Composed with the shared theme.** `kaicorp.css` and `landing-polish.css`
  ship as generated copies from the kaicorplabs repository and the pages use
  their composition. LinkUp keeps its own palette, as every tool of the house
  does; what is shared is the type and the frame.
- **Phones.** Under 640 px the links table becomes a stack of cards with the
  actions in reach, the inline forms in Settings stack instead of squeezing the
  field, the header no longer breaks the username in half, and nothing you can
  press is under 36 px tall — 44 on touch screens. Emoji left the buttons,
  headings and labels.

### Fixed

- Every page was titled "Dashboard", including the front page an anonymous
  visitor saw; the favicon was an emoji rendered by whatever font the visitor
  had.

## [0.1.5] — 2026-09-02

### Fixed

- **The preview page showed a signed-out header to someone who was signed in.**
  It passed an empty session to the template on purpose — the page is public and
  always will be — but that also decides what the header draws, so arriving from
  your own dashboard looked like the session had dropped. The PIN and error
  pages had the same omission. Reading the session there decides nothing about
  access; it decides whose name appears in the corner.

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
