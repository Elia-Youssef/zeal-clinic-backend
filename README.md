# Zeal Clinic Backend

## Requirements

- Go 1.26.1+
- Frontend build output in `client/dist` before release builds
- Inno Setup 6 only when building the Windows installer

## Build

Makefile shortcuts:

```bash
make dev            # build and run as --dev (binary in tmp/)
make dev-seed       # build and run as --dev --seed-only
make dev-demo       # build and run as --dev --seed-only --demo
make dev-cloud      # build cloud-tagged and run as --dev

make frontend       # npm build the frontend and copy dist into client/dist

make build          # debug build to tmp/ZealClinic_dev.exe
make build-cloud    # native cloud-tagged debug build to tmp/ZealClinicCloud_dev.exe
make release        # prod build to build/local/output/ZealClinic.exe (run `make frontend` first)
make release-cloud  # Linux cloud prod build to build/cloud/output/
make installer      # release + Inno Setup installer (Windows only)
make deploy         # frontend + release-cloud + installer

make clean          # remove build outputs and tmp/
```

`make frontend` builds `../zeal-clinic-frontend` by default and copies its
`dist` output into `client/dist`.

## Configuration

No runtime `.env` is read. Config is embedded per build tag: the `local` build
embeds `internal/config/local.env.defaults`, the `cloud` build (`-tags cloud`) embeds
`cloud.env.defaults`. Real process environment variables still override the baked-in defaults.

| Variable         | Default                          | Notes                                            |
| ---------------- | -------------------------------- | ------------------------------------------------ |
| `PORT`           | `55555` (local) / `8080` (cloud) | API and frontend port                            |
| `JWT_SECRET`     | baked-in (embedded env file)     | HMAC signing key                                 |
| `JWT_LIFETIME`   | `14h`                            | Invalid values fall back to `14h`                |
| `PEER_URL`       | empty                            | Local server's cloud peer URL (empty = no sync)  |
| `SYNC_SECRET`    | empty                            | Enables cloud `/api/sync/*` routes               |
| `PUBLIC_URL`     | empty                            | Cloud build advertises this at `/api/server-url` |
| `PUBLISH_SECRET` | empty                            | Enables cloud `POST /api/versions`               |
| `SENTRY_DSN`     | empty                            | Error tracking endpoint                          |

> `DB_ENCRYPTION_KEY` is intentionally unused — the at-rest key is a hardcoded constant.

DB path:

- Installed Windows: `%LOCALAPPDATA%\Zeal Clinic\Data\clinic.db` if that directory exists
- Otherwise: `./tmp/clinic.db`

## Layout

- `cmd/server`: flags, startup, sync, monitor, tray lifecycle
- `internal/api`: Echo server, routes, handlers, middleware
- `internal/database`: SQLite open, goose migrations, demo seeding
- `internal/database/store`: models and SQL operations
- `internal/sync`: transactional-outbox local/cloud replication
- `internal/monitor`: periodic jobs
- `internal/pdf`: invoice and report PDF generation
- `internal/realtime`: in-memory SSE hub
- `internal/tracking`: Sentry error tracking
- `client`: embedded Vite build
- `build/local`: Windows installer script/assets

## Tests

```bash
go test ./...
```
