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

The tag workflow builds one OCI directory layout with SBOM and provenance. Trivy
v0.74.0 reads OCI layouts as directories, not OCI tarballs. It scans that layout
before anything is uploaded. Skopeo copies the same layout with
`--all --preserve-digests`, retaining attestations and refusing digest changes.
The publisher checks the BuildKit digest before uploading and each destination
digest afterwards. It does not build a second image after the security gate.
Skopeo is a temporary CI tool, not a production container or runtime dependency.

Both PR CI and tag releases call `.github/actions/scanned-oci`. The PR job has
only `contents: read`, does not log in to a registry and cannot publish packages.
After tests it builds, scans and verifies the layout digest, retaining that OCI
directory as a GitHub artifact for three days. Its digest is in the job summary.
That artifact is for inspection/isolated staging, not approval to promote F1:
equivalent HTTP performance, final-image tests and rollback remain separate gates.
Docker's compiler and runtime bases are pinned by digest as well as version;
updating a tag without its digest does not change the selected image.

`scripts/publish-scanned-image.sh --verify-only` validates a prepared layout
and tags without contacting the registry. Set `RELEASE_LAYOUT`,
`RELEASE_DIGEST`, `RELEASE_REPOSITORY` and newline-separated `RELEASE_TAGS`.
Publishing without that flag requires an existing Docker login configuration;
never pass registry tokens in command arguments.

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
