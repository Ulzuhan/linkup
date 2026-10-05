# LinkUp agent guidance

Read `CONTRIBUTING.md`, `DEPLOYMENT.md` and `release/README.md` for this task.
Keep printed slugs, owners, Unix-second timestamps, redirect semantics and
SQLite/session/key revocations intact. Never reset live data, restore an old
database silently, read real credentials, or run two application writers against
one database. Backups include credentials and require private handling.

The reviewed 0.8.x image-only lane freezes production Go in
`release/persistence-policy.json`. Persistence/startup/authentication changes
require explicit manual compatibility and migration review. Reuse the scanned
OCI: functional CI and exact runtime tests precede publication, and the publisher
copies the tested bytes without rebuilding. Tags/releases/publication and live
deployment require explicit authorization; preparation and CI are not activation.

Use isolated branches and synthetic databases for tests. Preserve concurrent
work and the original checkout. Do not change Go dependencies, application
behavior, UI or production configuration as part of deployment preparation.
