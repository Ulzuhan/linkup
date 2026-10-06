# Exact OCI preparation and corrected contracts

The reviewed manifest authorizes only the stable 0.8.1 bootstrap publication.
It does not authorize host deployment or activation.
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

## The historical baseline is not an automatic return target

`rollback.json` still records the real signed0.8.0 release for a historical
negative control. It has the same DDL but does not enforce typed subject ownership
and its router-only health response lacks the SQLite marker. Exact-image CI
proves these incompatibilities, real new-key routes, SQLite failures and current
synthetic data preservation. It does not call that old pair compatible.

`bootstrap-policy.json` is a separately reviewed manifest. It currently has
`publication_authorized=true`, `version="0.8.1"`, `automatic_return=false` and
`floating_tags=false`. `persistence-policy.py --publication` requires the same
version in the Go repository's `VERSION` file and an exact canonical tag-push
context before registry login and candidate/promote registry access. OCI
verification also requires that exact version label. CLI flags and environment
cannot change the reviewed manifest. This is a
supervised first-publication lane, not a compatible automatic return lane.
The OCI declares `deployment-lane=supervised-bootstrap-v1` and
`automatic-return=false`; it must have no `rollback-image` label. Changing those
labels requires a new OCI build and rehearsal. The publisher copies that exact
tested index to a unique run/attempt candidate, verifies its signed source/tag,
certificate run/attempt and subject/digest, then copies only the version tag.
It never moves `0.8`, `0` or `latest`. The old shell helper is verification-only.
The infra wrapper likewise blocks adoption/apply/reconcile/rollback until a
corrected signed baseline and supervised bootstrap are separately reviewed.
Testing two unpublished corrected OCI artifacts can prove isolated image return;
fixture admission is not a real signature, published release or activation.

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

The infra first-jump rehearsal uses the signed historical B0 as origin and this
exact unpublished OCI as B1. B0 cannot be last-good for typed keys/readiness.
The supervised controller journals forward-only recovery and preserves current
SQLite; failures do not activate B0 or restore a backup. Unpublished B1 admission
is an explicit fixture. Publication, live cutover and eventual handoff to a
signed B1 baseline each require their own reviewed evidence. Real key reissue
is needed only when historical stored keys exist; a zero-key aggregate is not
an exhaustive inventory of external consumers or proof of database integrity.
Later automatic image updates still need a real compatible signed pair.
