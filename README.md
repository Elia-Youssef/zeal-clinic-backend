# Zeal Clinic Backend

## Requirements

- Go 1.26.1+
- Frontend build output in `client/dist` before release builds
- Inno Setup 6 only when building the Windows installer

## Build

Makefile shortcuts:

```bash
make dev            # build and run as --dev (binary in tmp/)"
make dev-seed       # build and run as --dev --seed-only"
make dev-demo       # build and run as --dev --seed-only --demo"

make frontend       # npm build the frontend and copy dist into client/dist"

make run            # build and run as prod (dev binary)"
make build          # debug build to tmp/ZealClinic_dev.exe"
make release        # prod build to build/local/output/ZealClinic.exe + embed frontend"
make installer      # release + Inno Setup installer (Windows only)"

make build-cloud    # cloud build to build/cloud/output/ZealClinicCloud-0.1.0-linux-amd64 (no tray, no browser)"
make release-cloud  # cloud prod build + embed frontend (run from WSL)"

make clean          # remove build outputs and tmp/"
```

`make frontend` builds `../zeal-clinic-frontend` by default and copies its
`dist` output into `client/dist`.

## Configuration

`.env` is loaded from the current directory first, then from the executable
directory.

| Variable            | Default                | Notes                               |
| ------------------- | ---------------------- | ----------------------------------- |
| `PORT`              | `55555`                | API and frontend port               |
| `JWT_SECRET`        | `dev-only-clinic-jwt-secret-not-for-release` | Change before shipping              |
| `JWT_LIFETIME`      | `14h`                  | Invalid values fall back to `14h`   |
| `DB_ENCRYPTION_KEY` | required               | Required by config; see drift below |
| `PEER_URL`          | empty                  | Local server's cloud peer URL       |
| `SYNC_SECRET`       | empty                  | Enables cloud `/api/sync/*` routes  |
| `PUBLIC_URL`        | empty                  | Cloud build advertises this in `/server-url` |

DB path:

- Installed Windows: `%PROGRAMDATA%\Zeal Clinic\clinic.db` if that directory exists
- Otherwise: `./tmp/clinic.db`

## Layout

- `cmd/server`: flags, startup, sync, monitor, tray lifecycle
- `internal/api`: Echo server, routes, handlers, middleware
- `internal/database`: SQLite open, goose migrations, demo seeding
- `internal/database/store`: models and SQL operations
- `internal/sync`: transactional-outbox local/cloud replication
- `internal/monitor`: periodic jobs
- `internal/pdf`: invoice and report PDF generation
- `client`: embedded Vite build
- `build/local`: Windows installer script/assets

## Tests

```bash
go test ./...
```
