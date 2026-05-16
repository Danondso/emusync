# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Emusync is a self-hosted emulator save file synchronization service for Linux desktops and Steam Decks. It has two components:
- **Server**: HTTP API daemon (runs in Docker) that stores versioned saves with conflict detection
- **Client**: CLI agent that monitors emulator processes and auto-syncs saves on launch/exit

## Build & Test Commands

```bash
make build            # Build binary with version from git tags
make test             # Run all tests (go test ./... -v)
make test-e2e         # Docker Compose E2E (go test -tags e2e ./tests/e2e/...)
make docker           # Build Docker container (docker compose build)
make docker-up        # Start server container (docker compose up -d)
make docker-down      # Stop server container (docker compose down)
make install          # Install binary to ~/.local/bin/
make install-service  # Install systemd user service
make init             # Generate default config.toml
make bootstrap-server # Run ./scripts/bootstrap-server.sh
```

CI (GitHub Actions): `staticcheck`, `go vet`, `go test ./...`, then Docker E2E (`-tags e2e`, 30-min timeout). Go version comes from `go.mod` (`go 1.26.0`).

Run a single test:
```bash
go test ./internal/hasher/ -v -run TestHashName
```

## Architecture

### Package Layout

- `main.go` — Entry point, delegates to `cmd.Execute()`
- `cmd/` — Cobra commands: `server`, `watch`, `push`, `pull`, `status`, `history`, `resolve`, `init`
- `internal/config/` — TOML config loading; `defaults.go` defines 19 default emulator mappings (RetroArch, Dolphin, PCSX2, RPCS3, Ryujinx, etc.)
- `internal/model/` — Shared types: `Manifest`, `FileEntry`, `Conflict`, `SyncResult`, `EmulatorConfig`
- `internal/server/` — HTTP server, file storage (`storage.go`), auth middleware (`auth.go`), conflict handling, admin UI (`admin.go`, `adminweb/`)
- `internal/client/` — `Syncer` (push/pull orchestration, state tracking) and `APIClient` (HTTP with retries)
- `internal/watcher/` — Process monitor: `/proc` polling (`watcher.go`), Flatpak/bwrap extraction (`flatpak.go`), Proton/Wine detection (`proton.go`)
- `internal/hasher/` — Concurrent SHA-256 file hashing with worker pool (NumCPU workers)
- `internal/logging/` — `slog`-based structured logging to stderr + file (`~/.local/share/emusync/sync.log`)
- `internal/authtoken/` — Token normalization (strips quotes/whitespace, logs warning if transformed)
- `internal/discovery/` — mDNS advertisement (`_emusync._tcp`) and lookup; optional, enabled via `EMUSYNC_ADVERTISE_MDNS=true`
- `internal/setup/` — Interactive CLI wizard for config generation

### Dependencies

Three external dependencies: **Cobra** (CLI framework), **BurntSushi/toml** (config parsing), **hashicorp/mdns** (service discovery). Everything else uses Go standard library.

### Key Patterns

**Conflict detection**: Client sends `X-Base-Hash` (last-synced SHA-256) on upload. Server returns HTTP 409 if it doesn't match the current canonical hash. Conflicts accumulate in the result — they don't abort sync. All files continue processing; conflicts stored in `/data/conflicts/` for later resolution.

**Sync serialization**: `Syncer.mu` mutex serializes all sync operations end-to-end. Launch and exit events can never interleave state mutations. Goroutines for post-exit sync are spawned independently to avoid blocking the watcher, but each acquires the mutex for the full operation.

**State tracking**: Client persists last-synced SHA-256 hashes in `~/.local/share/emusync/state.json`. Server manifest is authoritative; state.json is used only for delta detection (skip unchanged files). If state is stale or missing, server state takes precedence on next sync.

**Atomic writes**: Both client and server write to `.tmp` then `os.Rename()` to prevent partial-file corruption. Applied to canonical files, metadata JSONs, conflict storage, and the state file.

**Process monitoring**: `Watcher` polls `/proc` at configurable interval (default 2000ms). On Launched: `SyncBeforeLaunch` (pull). On Exited: wait `post_exit_delay_ms` (default 2000ms, to allow save flush), then `SyncAfterExit` (push). Wrapper detection handles Flatpak (`comm == "bwrap"`, extract binary after `--` separator), Proton/Wine (`.exe` extraction from `steamapps/common/Proton` paths), and AppImage (suffix stripping). All name matching is case-insensitive.

**Retry logic**: `APIClient.do()` retries up to 3 times on connection failure with linear backoff (`attempt * 1s`). Only retries requests where body can be replayed (`Body == nil` or `GetBody != nil`). File uploads (PUT with streaming body) are not retried.

**Path traversal safety**: `storage.go:validatePath()` uses `filepath.Rel()` + `strings.HasPrefix()` to ensure paths resolve within the data directory. Client syncer applies the same check before writing downloaded files.

### Server API (v1)

```
GET  /api/v1/manifest/{emulator}         — File list with metadata
GET  /api/v1/files/{emulator}/{path...}  — Download file (headers: X-SHA256, X-Timestamp, X-Device-ID)
PUT  /api/v1/files/{emulator}/{path...}  — Upload file (req headers: X-Device-ID, X-SHA256, X-Timestamp; optional: X-Base-Hash)
GET  /api/v1/conflicts                   — List unresolved conflicts
POST /api/v1/conflicts/{id}/resolve      — Resolve conflict (body: {"choice": "local"|"remote"|"keep-both"})
GET  /api/v1/history/{emulator}/{path}   — Version history
GET  /api/v1/health                      — Health check
```

### Server Storage Layout

```
/data/
├── canonical/{emulator}/       # Latest version of each file
├── backups/{emulator}/         # Versioned history (.bak.{timestamp}), rotated by EMUSYNC_MAX_BACKUPS
├── metadata/{emulator}/        # FileEntry JSON metadata per file
└── conflicts/                  # {id}.json (metadata) + {id}.local (incoming file)
```

### Configuration

Client: TOML at `~/.config/emusync/config.toml`. See `deploy/config/config.example.toml`. Key defaults applied by `config.go:applyDefaults()`: `saves_path` → `~/Emulation/saves`, `poll_interval_ms` → 2000, `post_exit_delay_ms` → 2000, `conflict_strategy` → `"prompt"` (also: `"newest"`, `"keep-both"`), `max_local_backups` → 10. `device_id` is required.

Server: env vars only — `EMUSYNC_PORT`, `EMUSYNC_DATA_DIR`, `EMUSYNC_AUTH_TOKEN`, `EMUSYNC_ADMIN_TOKEN`, `EMUSYNC_MAX_BACKUPS`, `EMUSYNC_ADVERTISE_MDNS`. Token env vars are normalized (quotes/whitespace stripped) by `authtoken.Normalize()`.
