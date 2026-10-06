# Exact OCI preparation and corrected contracts

The 0.8.2 candidate prepares an image-return contract with the deployed, signed
0.8.1 baseline. Production Go, dependencies, UI, Dockerfile and SQLite schema
are unchanged. This maintenance candidate supplies the reviewed release bytes
needed to rehearse B1/C; it adds no application feature. Publication, merge,
host handoff and activation are pending approval.
Functional CI builds and scans one linux/amd64 OCI, executes its hashed runtime,
archives its provenance/SBOM and verifies transferred bytes. Trivy still blocks
fixable HIGH/CRITICAL vulnerabilities. No image is rebuilt at publication.

The production Go file set and hashes in `persistence-policy.json` freeze this
manual auth/readiness correction (`linkup-auth-readiness-v2`). Changed writers,
startup, SQL or auth still require manual compatibility review. The SQLite DDL,
link owners/slugs, Unix seconds and session encryption format are unchanged.

## API key identity and compatibility

New OIDC keys created by `/api/keys` or `/settings/keys` receive a server-generated
`oidc-subject-v2:` ID and retain the verified subject as `user_id`. Authorization
looks up only that subject's unexpired login, checks UserInfo and the available group projection,
then revalidates the exact key ID/hash/owner and session after the provider
request. Group membership never elevates a key to group administrator. The
historical login migration cannot reassign typed keys to a matching mutable name.

Historical OIDC keys have no durable distinction between subject and login-name
ownership. They now fail closed and must be reissued from a verified cookie;
this includes old subject keys. Existing real-login adoption retains historical
metadata management, but does not make untyped keys usable in OIDC mode.
Standalone key authentication retains its existing behavior. No bulk bearer
migration, new DDL, broad alias access or changes to the raw secret/hash format.
Already authorized in-flight operations are not undone by later revocation.

## Liveness and readiness

`/health` is process HTTP liveness. `/healthz` is readiness and returns200 only
with `{"status":"healthy","service":"linkup","sqlite":"ready"}`. Failure is a
generic503, with no database path, SQL, rows, provider body or internal error.
Both existing routes are outside the write limiter; no printed slug is reserved
or shadowed by a new top-level `/livez` or `/readyz` alias.

Readiness records the application DB.Close lifecycle, checks the configured
regular file's original inode and opens one fresh `file:` SQLite URI with
`mode=ro`, `query_only(1)` and busy_timeout100ms. It reads schema metadata and
prepares all required table/column projections with `WHERE0`; no user rows are
read and no user data is written. It never calls the writer's Open/Migrate/Ping
path, recreates a missing file, or restores anything. SQLite WAL reader bookkeeping
may touch SHM read marks. A single global probe and a one-second context budget
bound contention; modernc Prepare/open I/O does not guarantee a hard OS I/O
deadline. This does not prove write access, free space, constraints/indexes or full
page/row integrity. Backups require separate integrity/schema checks.

## Prepared compatible image return, with publication blocked

`rollback.json` records the real signed B1 release 0.8.1: source
`39e284e695cecb8671f79a710ef187385b9843c5`, publisher 37453232322/attempt1,
OCI `sha256:8bcc1f3874ec74b5eb1a9704002388ce0a3d63fdee98b47b277e1e79a80e60d5`.
The workflow verifies the successful release record and exact signature
certificate source/run/attempt/subject/digest, and executes its actual runtime
`sha256:643a0da53feb3e2e6868f30499eb214f2da5a4602de526f3200cab78ab963d0f`.
B1 keeps its original bootstrap labels and has no rollback-image label.
Historical 0.8.0 is incompatible with typed keys and SQLite readiness and is
never selected as the compatible return image.

The existing reviewed `bootstrap-policy.json` now selects the separate
`compatible-image-return-v1` lane, version0.8.2, `automatic_return=true`,
`publication_authorized=false` and `floating_tags=false`. In this lane,
automatic-return expresses a technical capability: return only the image to
the exact B1 digest over current SQLite, with the same configuration and one
application writer at a time. It grants no operational permission. The OCI
must declare that lane, capability and exact `rollback-image` B1. Actual B1
labels must not be rewritten to imitate C.

The publication gate refuses even an exact canonical version-tag push while
publication_authorized is false, before registry login or candidate/promote
registry access. CLI/environment overrides cannot authorize it. The direct
shell publisher remains verification-only. CI stores the scanned OCI as an
Actions artifact; it creates no registry candidate, version tag, release or
floating alias. A future authorized publisher must reuse verified bytes and
bind source/tag/run/attempt/digest; any source/policy change needs a new CI OCI
and rehearsal. It never moves `0.8`, `0` or `latest`.

Exact-image CI exercises B1 -> C -> B1 on synthetic current SQLite. It tests
real key creation/revocation routes, a C-created typed key usable after B1
return, writes and revocations, readiness and a separate stale-restore negative
control. The infra Compose rehearsal reuses that same OCI without rebuild.
C release admission is explicitly simulated there; B1 signature is real.
The infra wrapper keeps BOOTSTRAP_REQUIRED=True and installed LinkUp timers
remain absent/inactive. Handoff and operation require later approval.

Backups contain hashed keys, encrypted access tokens, sessions and webhook
secrets and require private handling. A consistent backup includes committed
WAL. Image return uses current SQLite with the same config/session key and one
writer at a time. A separate stale-restore control shows lost new writes and
revived deleted auth. Data restore is always an explicit manual recovery,
never part of automatic image rollback. Async clicks/webhooks and downtime
remain outside the guarantee;0.8.0 also lacks the current cache locking fix.

## Permission projection and bootstrap rehearsal

The synthetic Go callback fixture explicitly covers bare UserInfo and empty
groups with the same signed JWT: access and cookie administration continue
using the JWT's old groups until the provider rejects the token or the local
session expires/is revoked. A fresh non-empty group list without the required
group denies access. API keys do not gain group administration in either case.
This bounds current behavior; consulting UserInfo alone does not prove fresh
group membership. Supabase Auth v2.197.0 checks JWT/user/session for UserInfo,
but does not check revoked consent in that route. The deployed account admin
path revokes consent without deleting provider sessions. This fallback is
unchanged from the signed B0 source; it is not a new B1 regression. Its
conditional impact and unmeasured runtime limits are tracked in the separate
[OIDC follow-up](oidc-followups.md). A real permission-change test is not a
default CD prerequisite. Stronger revocation guarantees need their own reviewed
contract and validation; the typed-key/readiness and deployment gates remain.

The completed first jump used signed historical B0 -> signed B1 with
forward-only recovery over current SQLite. It must not be repeated as a normal
rollback. This candidate instead prepares the compatible B1/C pair in isolation;
the live bootstrap journal stays intact until a separately approved handoff.
Real key reissue is needed only when historical stored keys exist; a zero-key
aggregate is not an exhaustive inventory of external consumers or proof of
database integrity. The canceled real OIDC permissions audit is outside CD.
