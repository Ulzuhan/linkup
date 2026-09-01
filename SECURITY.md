# Security policy

LinkUp handles redirects, PINs, API keys and OIDC sessions. If you find a way to
break any of that, please tell us before telling the internet.

## Reporting a vulnerability

Open a [private security advisory](https://github.com/Ulzuhan/linkup/security/advisories/new)
on this repository. That channel is private until we publish it together.

Please do not open a public issue for a vulnerability.

**What to expect:** an acknowledgement within 72 hours, an assessment within 7
days, and a fix or a written explanation of why it is not one. You will be
credited in the advisory unless you would rather not be.

## What counts

In scope, and taken seriously:

- Anything that makes the server issue a request to an address it should refuse
  (server-side request forgery).
- Reaching links, PIN-protected destinations, API keys or webhooks belonging to
  another account.
- Forging or tampering with a session cookie or an API key.
- Anything that stores or exposes visitor data the privacy doctrine says is
  never collected — an address, a User-Agent, a referrer.
- Injection into the dashboard or the redirect path.
- Turning an instance into an open redirect for phishing.

Out of scope:

- Findings that require the operator to have deliberately weakened the
  instance — `LINKUP_DEV_MODE` outside loopback (which refuses to start),
  `LINKUP_ALLOW_PRIVATE_TARGETS` (which is documented as a choice), or a
  publicly exposed port with no reverse proxy.
- Missing hardening headers on an endpoint that returns no content.
- Volumetric denial of service. The public redirect has no per-visitor limit by
  design; that belongs at the edge.
- Reports from automated scanners without a working demonstration.

## Design constraints worth knowing before you test

- **No visitor address is ever read.** That is a product promise, and it removes
  the usual per-IP rate limit. Writes are budgeted per authenticated identity,
  and PIN attempts per link rather than per visitor. If you find a way to abuse
  something that has no budget, that is a valid report.
- **User-Agent is inspected in flight** for device routing and is never stored.
  Finding it persisted anywhere is a valid report.
- **Webhook destinations are validated twice**, when stored and again before
  delivery, on resolved addresses rather than on the hostname. A way past either
  check is a valid report.

## Supported versions

The latest tagged release. This is a young project; there are no backports yet.
