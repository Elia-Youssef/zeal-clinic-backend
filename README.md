# Zeal Clinic — Backend

Single Go binary for a beauty/dental clinic: an Echo REST API, an encrypted
SQLite database, generated PDFs, local↔cloud replication, and the embedded React
frontend — all served from one executable.

## Requirements

- Go 1.26+
- Frontend built into `client/dist` before any release build (`make frontend`)
- Inno Setup 6 — only for the Windows installer

## Run

```bash
make dev         # build + run in dev mode (no tray/browser, DB in ./tmp)
make dev-demo    # same, seeded with a large demo dataset
go run ./cmd/server --dev        # without the Makefile
```

Listens on `:55555` (local) / `:8080` (cloud). The seeded `admin` user has no
password and sets one on first sign-in.

## Build

`make help` lists every target. Common: `make build` (dev exe → `tmp/`),
`make release` (Windows prod), `make release-cloud` (Linux), `make installer`,
`make deploy`. `VERSION` is the single source of truth for the build version.

## Configuration

No runtime `.env`. Config is embedded per build tag from
`internal/config/local.env.defaults` (local) or `cloud.env.defaults` (`-tags cloud`); real
environment variables override the baked-in defaults.

| Variable | Notes |
| --- | --- |
| `PORT` | API + frontend port (`55555` local / `8080` cloud) |
| `JWT_SECRET` / `JWT_LIFETIME` | JWT signing key / TTL (default `14h`) |
| `PEER_URL` + `SYNC_SECRET` | clinic's cloud peer + shared secret (both set = sync on) |
| `PUBLIC_URL` / `PUBLISH_SECRET` | cloud `/api/server-url` / enable `POST /api/versions` |

The at-rest DB encryption key is a hardcoded constant (intentional). DB path:
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
