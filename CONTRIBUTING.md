# Contributing to LinkUp

## Getting it running

Go 1.24 or newer. No CGO, no system libraries — the SQLite driver is pure Go.

```bash
cp .env.example .env          # then set LINKUP_SESSION_SECRET
make run                      # http://127.0.0.1:3464
```

For local work without an identity provider, set `LINKUP_DEV_MODE=true` and
leave the OIDC variables empty. Every request without a cookie is then treated
as an administrator, which is why the server **refuses to start in that mode
unless it is bound to loopback**. If you see it abort, that guard is doing its
job.

## Before you open a pull request

```bash
make test        # go test -race ./...
gofmt -l .       # must print nothing
go vet ./...
```

CI runs the same three plus a build. It is not decoration: a red CI once hid a
missing entrypoint for a whole day.

## Conventions

- **Commits** follow [Conventional Commits](https://www.conventionalcommits.org/):
  `fix(security): …`, `feat(auth): …`, `docs: …`. Explain *why* in the body. A
  diff already says what changed.
- **Tests** live in `tests/` as a black-box package. The exception is behaviour
  that can only be exercised from inside — the DNS resolver seam in
  `internal/services/egress_internal_test.go` is the one case, and it is
  commented as such. Do not export something purely to satisfy the convention.
- **Comments explain the reason, not the mechanism.** `// increment counter` is
  noise; `// counted per link and not per visitor, because we never look at a
  visitor's address` is the kind that survives.
- **Architectural decisions** go in `docs/decisions/` as an ADR. If a change
  makes someone ask "why is it like this?", it needs one.

## Things that will be pushed back on

- **Reading the visitor's IP address, User-Agent or referrer into storage.**
  This is the product's central promise. Anything that needs per-visitor state
  needs a different design, not an exception.
- **A user-supplied URL reaching an HTTP client without passing
  `ValidateOutboundURL`.** That is how the SSRF fixed in `576be13` happened.
- **New dependencies** without a line in the PR saying what they replace. The
  dependency tree is small on purpose; it is half the reason this is written in
  Go (see [ADR 0001](docs/decisions/0001-go-for-the-redirect-engine.md)).
