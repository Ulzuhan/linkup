# Contributing to LinkUp

## Getting it running

The release and CI toolchain is Go **1.27.1**, also selected by the `toolchain`
directive in `go.mod`. The language minimum remains 1.25 for comparison and
compatibility, not as a recommendation to deploy an unsupported compiler.
CI and Docker use `GOTOOLCHAIN=local` so they cannot silently switch compilers.
No CGO in the release binary — the SQLite driver is pure Go. Race tests require
a CGO-capable development/CI environment.

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
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
```

CI runs these checks plus a build and the read-only OCI verification below.
It is not decoration: a red CI once hid a
missing entrypoint for a whole day.

## Reproducible authorization microbenchmarks

```bash
GOMAXPROCS=1 GOTOOLCHAIN=go1.27.1 go test -p 1 -run '^$' -bench BenchmarkLiveOIDC -benchtime=1s -count=3 ./internal/services
```

Fixtures use a temporary SQLite database and a signed HTTP test identity provider.
Every operation still queries UserInfo; no permission cache is introduced.
Compare against the same source using the previous compiler, alternate run order,
and record host load. These measure ns/op and allocations, not container RSS,
production p95, or a complete browser journey. Never target a production IdP.
The test binary recompiles the application, fake IdP and client together: its
change is not attributable solely to LinkUp. Image-to-image HTTP comparisons
must keep the external test IdP and load generator fixed.

## Conventions

### Release artifact

The tag workflow calls functional CI at the exact release SHA. CI calls
`.github/actions/scanned-oci` once, scans the OCI layout with the existing
Trivy v0.75.0 policy, loads its exact config ID and tests corrected key routes, SQLite readiness and historical data return over
current synthetic SQLite, with the old automatic pair explicitly blocked. Source/digest and the archive are retained for seven
days under `linkup-oci-<run-id>` and reverified after transfer. Only the tag
publisher gets registry/signing permissions after these gates; it copies the
same bytes, verifies signed source/tag/run/attempt and promotes only the reviewed
version without rebuilding or moving floating aliases. The bootstrap manifest
remains disabled; synthetic publisher tests never authorize real publication.

The rehearsal uses CI-only `python3-cryptography` from the runner's distribution
to generate synthetic AES-GCM cookies matching the existing Go format; it adds
no application dependency. It proves why stale backup restore must remain
manual and separate from image return. Functional/return checks do not approve
performance promotion: the historical F1 evidence and its limits remain separate.
See [the reviewed lane and limits](release/README.md).
Docker's compiler and runtime bases are pinned by digest as well as version;
updating a tag without its digest does not change the selected image.
The [F1 OCI evidence](docs/benchmarks/2026-09-05-f1-oci.md) records a verified
CI layout, linked SPDX/SLSA statements and the final binary compiler/hash;
it explicitly does not approve performance or production promotion.

`scripts/publish-scanned-image.sh --verify-only` validates a prepared layout
and tags without contacting the registry. Set `RELEASE_LAYOUT`,
`RELEASE_DIGEST`, `RELEASE_REPOSITORY` and newline-separated `RELEASE_TAGS`.
Direct publication through that helper is disabled. The canonical publisher is
`oci-release.py`, gated by the reviewed bootstrap manifest. Never pass registry
tokens in command arguments.

### Code and review

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
