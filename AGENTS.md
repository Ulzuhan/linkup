# LinkUp - Agent & Contributor Guidelines

## Architecture & Code Conventions

- **Language**: Go 1.24+ standard idioms.
- **Routing**: `chi` router (`github.com/go-chi/chi/v5`).
- **Database**: SQLite (via `modernc.org/sqlite` pure Go) with WAL mode.
- **UI & Templates**: Handled by `html/template` and embedded via `embed.FS` in `internal/web/`.
- **Privacy Doctrine**: Never log visitor IP addresses, User-Agents, or Referrers to persistent storage or stdout logs.

## Verification

Before pushing, always ensure:
```bash
make test
make bench
go vet ./...
gofmt -l .
```
