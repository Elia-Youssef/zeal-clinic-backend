# Zeal Clinic — Backend

Single Go binary for a beauty/dental clinic: an Echo REST API, an encrypted
SQLite database, generated PDFs, local↔cloud replication, and the embedded React
frontend — all served from one executable.

## Requirements

- Go 1.26.8 or newer (the standard library security fixes need it; with the default `GOTOOLCHAIN=auto`, the go command fetches the matching toolchain on its own when yours is older)
- Frontend built into `client/dist` before any release build (`make frontend`, from the sibling `../zeal-clinic-frontend` checkout; `FRONTEND=<path>` overrides)
- Inno Setup 6 — only for the Windows installer

## Run

```bash
make dev         # build + run in dev mode (no tray/browser, DB in ./tmp)
make dev-demo    # same, seeded with a large demo dataset
go run ./cmd/server --dev        # without the Makefile
make demo        # one node with demo data, Windows (or: pwsh scripts/demo.ps1)
pwsh scripts/demo-two-node.ps1   # clinic and cloud side by side, Windows
scripts/demo-cloud.sh            # cloud node on Linux (make demo runs it there); money is read-only without its clinic
# demo sign-in: jvance (admin), tmercer (staff), lhayes and mowens (nurses); password demo123
```

Listens on `:55555` (local) / `:8080` (cloud). The seeded `admin` user has no
password and sets one on first sign-in.

## Build

`make help` lists every target. Common: `make build` (dev exe → `tmp/`),
`make release` (Windows prod), `make release-cloud` (Linux), `make installer`,
`make deploy`. `VERSION` is the single source of truth for the build version.

## Configuration

No runtime `.env`. Config is embedded at build time, per build tag, from
`internal/config/`:

- `local.env.defaults` (local) and `cloud.env.defaults` (`-tags cloud`):
  committed dev defaults, so a fresh clone builds and runs as is. Their secrets
  are public dev values.
- `local.env` / `cloud.env`: optional overrides with the real values,
  git-ignored. A non-empty value replaces the default of the same key; start
  from `local.env.example` / `cloud.env.example`.

| Variable | Notes |
| --- | --- |
| `PORT` | API + frontend port (`55555` local / `8080` cloud) |
| `JWT_SECRET` / `JWT_LIFETIME` | JWT signing key, 32+ characters / TTL (default `14h`) |
| `PEER_URL` + `SYNC_SECRET` | clinic's cloud peer + shared secret (both set = sync on) |
| `PUBLIC_URL` / `PUBLISH_SECRET` | cloud `/api/server-url` / enable `POST /api/versions` |
| `DB_ENCRYPTION_KEY` | at-rest DB encryption key, 64 hex characters |

The local and cloud builds must share `SYNC_SECRET` and `DB_ENCRYPTION_KEY`.
A stamped release build refuses to start with a dev default, a `JWT_SECRET`
under 32 characters or a malformed `DB_ENCRYPTION_KEY`, and `make release` /
`make release-cloud` run the same checks on the override files before building
(`go run ./cmd/releasecheck clinic cloud` checks both). DB path:
installed Windows → `%LOCALAPPDATA%\Zeal Clinic\Data\clinic.db`; cloud →
`./data/clinic.db`; dev → `./tmp/clinic.db`.

## Layout

| Path | Responsibility |
| --- | --- |
| `cmd/server` | entrypoint, flags, startup & shutdown lifecycle |
| `internal/api` | Echo server, routes, handlers, middleware (auth, scopes, cache, audit) |
| `internal/database` + `.../store` | SQLite open, goose migrations, models & SQL |
| `internal/sync` | cloud↔local transactional-outbox replication |
| `internal/{monitor,pdf,realtime,updater,tracking}` | periodic jobs, PDFs, SSE, self-update, logging |
| `client` | embedded Vite build (`//go:embed`) |

Schema/ERD source: `docs/erd-dbdiagram.dbml`.

## Tests

```bash
go test ./...
```
