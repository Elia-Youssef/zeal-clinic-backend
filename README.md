# Zeal Clinic Backend

Go + SQLite backend with an embedded Vite frontend. Single binary; no external runtime.

---

## Prerequisites

- **Go 1.23+**
- **CGO enabled** (`go-sqlite3` requires it)
  - `go env -w CGO_ENABLED=1`
  - A C compiler on `PATH` — on Windows, grab a MinGW-w64 gcc build from [winlibs.com](https://winlibs.com), unzip somewhere (e.g. `C:\mingw64`), and add its `bin\` folder to your system `PATH`. Open a new terminal and verify with `gcc --version`.
- **Inno Setup 6** (only if building the Windows installer)

---

## Run (dev)

```bash
go run ./cmd/server                              # start on :8080 (auto-opens browser, runs in systray)
go run ./cmd/server --no-browser                 # start without auto-opening the browser
go run ./cmd/server --dev                        # WSL/Linux dev: no tray, no browser, db at ./dist/clinic.db
go run ./cmd/server --seed-only                  # seed defaults (roles, admin, currencies, catalog) and exit
go run ./cmd/server --seed-only --demo           # seed defaults + rich demo data and exit
```

Seeding only runs under `--seed-only` (the installer uses this post-install). A plain `go run ./cmd/server` never touches seed data. If another instance is already serving on the configured port, the process just opens the browser to it and exits.

---

## Build

Make sure the frontend build output is in [client/dist/](client/dist/) — it gets embedded into the binary at compile time.

```bash
# standard build (console window visible — logs go to stdout)
go build -o installer/ZealClinic.exe ./cmd/server

# release build for Windows click-to-run (no console window)
go build -ldflags "-H=windowsgui" -o installer/ZealClinic.exe ./cmd/server
```

`-H=windowsgui` hides the console. Logs written via `log.*` will be discarded; if you need logs in release builds, redirect them to a file from inside the app.

---

## Installer (Windows)

```bash
# 1. Build the binary next to the installer script
go build -trimpath -ldflags "-s -w -H=windowsgui" -o installer\ZealClinic.exe .\cmd\server

# 2. Compile the Inno Setup script
"C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\installer.iss
```

Output: `installer\output\ZealClinicSetup-<version>.exe`.

The installer:
- Puts `ZealClinic.exe` under `Program Files\Zeal Clinic\`
- Stores `clinic.db` under `%PROGRAMDATA%\Zeal Clinic\` (survives uninstall unless the user opts to delete)
- Generates a fresh `.env` next to the exe with a random `JWT_SECRET` (preserved across upgrades)
- Runs `ZealClinic.exe --seed-only` once post-install to populate the DB
- Adds Start Menu shortcut + optional desktop shortcut
- Adds/removes an inbound TCP 8080 Windows Firewall rule
- Stops any running instance before overwriting files on upgrade

---

## Configuration

All via env vars (read from `.env` if present — optional):

| Variable       | Default                 |
|----------------|-------------------------|
| `PORT`         | `8080`                  |
| `JWT_SECRET`   | `dev-only-clinic-jwt-secret-not-for-release`  |
| `JWT_LIFETIME` | `12h`                   |

The SQLite database is not configurable via env var. It lives at `%PROGRAMDATA%\Zeal Clinic\clinic.db` when that folder exists (installed layout), `./dist/clinic.db` under `--dev`, else `clinic.db` in cwd. `.env` is loaded from cwd first, then from the directory containing the executable — so installed deployments can be reconfigured by editing `.env` next to `ZealClinic.exe`.

**Change `JWT_SECRET` before shipping.** The default logs a warning on startup.
