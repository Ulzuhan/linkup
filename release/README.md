# Image-only return over current SQLite

This lane is prepared for review; it does not deploy, schedule work or authorize
publication. The initial compatible scope is patches in 0.8.x. The exact signed
0.8.0 baseline is recorded in `rollback.json`: source, digest, publisher run and
attempt are verified against the certificate, not a free provenance predicate.
That historical release has a tag and signed publisher even though no GitHub
Release object exists.

The release calls functional CI at its exact tag SHA. CI builds one scanned OCI,
loads its hashed amd64 runtime, exercises HTTP and SQLite return against the
baseline, and archives the same index including provenance and SBOM. Transfer
is reverified byte by byte. Only the tag publisher gets write permissions after
the gates; it copies those bytes, verifies its signature and then promotes the
same digest. PRs, branches, forks and manual runs cannot publish. Stable version
tags cannot be silently replaced with different bytes. Trivy's existing policy
still blocks fixable HIGH/CRITICAL findings.

`persistence-policy.json` freezes the production Go file set and bytes reviewed
at 31ddb9036b33315bce08ebefe8152044fc022b18. This is conservative: changed SQL,
startup, authentication, writers or newly added production Go require a manual
compatibility/migration review before changing the policy. A hash guard is not
a proof of every semantic property. Dependencies and static assets can change
only while all functional and exact-runtime gates still pass.

Persisted data consists of `linkup.db` and its WAL/SHM companions. Owners and
slugs, custom domains, folders, targets/routing, pause/expiry/click budgets and
Unix seconds retain their meanings. The database also contains hashed API keys,
encrypted OIDC access tokens, sessions, logout replay IDs and webhook secrets;
backups must be treated as credentials. Keep the same session encryption key
and configuration across an image return. Unreviewed associated files or a
foreign/partial/changed schema require coordinated backup and manual migration.

The synthetic gate uses one application writer at a time and a separate temporary
directory. A read-only SQLite source connection creates a consistent backup,
including committed WAL; integrity and schema are checked. Candidate writes,
updated targets, new slugs, pauses/deletions, counters and deleted session/key
rows survive the return to the exact previous image over the **current database**.
Synthetic encrypted cookies and keys work before deletion and remain rejected
after return. OIDC discovery/login/PKCE are checked against a disposable provider.

A negative control restores the older copy only into a new isolated target:
new writes disappear and formerly deleted cookies/keys become usable again.
Integrity success does not imply semantic preservation. No automatic data restore
exists. An operator must stop the writer, choose the recovery point, reconcile
newer writes and revocations, and explicitly approve any restore. Never overwrite
current SQLite as part of image rollback.

Limits: the gate does not authenticate a real account or exercise a live tunnel.
Health endpoints report router health; they do not themselves query SQLite, so
the rehearsal also verifies database reads/writes and integrity. Production
adoption requires a separately authorized consistent backup and health/resource
inspection. No live data or credentials are used here. Async click/webhook work
in flight is not guaranteed by the five-second shutdown; no downtime guarantee
is made. Returning to 0.8.0 also returns its older cache mutation behavior; the
current source's locking fix is not present in that historical binary.

Before a later release, review and test the rollback pair against the effective
last-good. Do not skip releases without proving that exact return pair. Format,
configuration, resource, session-key and migration changes remain manual.
