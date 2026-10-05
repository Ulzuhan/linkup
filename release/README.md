# Exact OCI preparation and corrected contracts

These drafts prepare LinkUp; they do not authorize publication or activation.
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
looks up only that subject's live login, checks UserInfo and current access,
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
negative control. It has the same DDL but cannot authorize typed subject keys
and its router-only health response lacks the SQLite marker. Exact-image CI
proves these incompatibilities, real new-key routes, SQLite failures and current
synthetic data preservation. It does not call that old pair compatible.

Publication is explicitly blocked by `persistence-policy.py --publication`,
both before registry login in the workflow and before candidate/promote copy.
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
