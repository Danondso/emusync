# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Emusync is a self-hosted emulator save file synchronization service for Linux desktops and Steam Decks. It has two components:
- **Server**: HTTP API daemon (runs in Docker) that stores versioned saves with conflict detection
- **Client**: CLI agent that monitors emulator processes and auto-syncs saves on launch/exit

## Build & Test Commands

```bash
make build          # Build binary with version from git tags
make test           # Run all tests (go test ./... -v)
make test-e2e       # Docker Compose E2E (go test -tags e2e ./tests/e2e/...)
make docker         # Build Docker container (docker compose build)
make docker-up      # Start server container (docker compose up -d)
make docker-down    # Stop server container (docker compose down)
make install        # Install binary to ~/.local/bin/
make install-service # Install systemd user service
make init           # Generate default config.toml
```

CI (GitHub Actions): `staticcheck`, `go vet`, `go test ./...`, then Docker E2E (`-tags e2e`).

Run a single test:
```bash
go test ./internal/hasher/ -v -run TestHashName
```

## Architecture

### Package Layout

- `main.go` — Entry point, delegates to `cmd.Execute()`
- `cmd/` — Cobra CLI commands: `server`, `watch`, `push`, `pull`, `status`, `history`, `resolve`, `init`
- `internal/config/` — TOML config loading with 19 default emulator mappings (`defaults.go`)
- `internal/model/` — Shared types: `Manifest`, `FileEntry`, `Conflict`, `SyncResult`, `EmulatorConfig`
- `internal/server/` — HTTP server, file storage, auth middleware, conflict handling
- `internal/client/` — `Syncer` (push/pull orchestration, state tracking) and `APIClient` (HTTP with retries)
- `internal/watcher/` — Process monitor using `/proc` parsing, with Flatpak/bwrap and Proton/Wine wrapper detection
- `internal/watchlock/` — Single-instance lock for `watch` (`~/.local/share/emusync/watch.lock`, Unix `flock`)
- `internal/hasher/` — Concurrent SHA-256 file hashing with worker pool
- `internal/logging/` — `slog`-based structured logging to stderr + file

### Key Patterns

**Conflict detection**: Client sends a base hash (last-synced) with uploads. Server returns HTTP 409 if the base hash doesn't match the current canonical hash. Conflicts are stored separately under `/data/conflicts/`.

**Process monitoring**: `Watcher` polls `/proc` at a configurable interval, emits `ProcessEvent` structs (Launched/Exited) on a channel. The `watch` command triggers pull-before-launch and push-after-exit. Supports Flatpak sandboxes, Proton/Wine wrappers, AppImage suffixes, and case-insensitive matching.

**Atomic writes**: Files are written to `.tmp` then renamed to prevent corruption (`storage.go:atomicWrite`).

**State tracking**: Client persists last-synced SHA-256 hashes in `~/.local/share/emusync/state.json` for delta sync.

### Server API (v1)

```
GET  /api/v1/manifest/{emulator}         — File list with metadata
GET  /api/v1/files/{emulator}/{path...}  — Download file
PUT  /api/v1/files/{emulator}/{path...}  — Upload file (sends base hash for conflict detection)
GET  /api/v1/conflicts                   — List unresolved conflicts
POST /api/v1/conflicts/{id}/resolve      — Resolve conflict
GET  /api/v1/history/{emulator}/{path}   — Version history
GET  /api/v1/health                      — Health check
```

### Server Storage Layout

```
/data/
├── canonical/{emulator}/   # Latest version of each file
├── backups/{emulator}/     # Versioned history
├── metadata/{emulator}/    # FileEntry JSON metadata
└── conflicts/              # Conflicting files + metadata
```

### Configuration

TOML config at `~/.config/emusync/config.toml`. See `deploy/config/config.example.toml` for reference. Server uses env vars: `EMUSYNC_PORT`, `EMUSYNC_DATA_DIR`, `EMUSYNC_AUTH_TOKEN`, `EMUSYNC_MAX_BACKUPS`.

## Dependencies

External libraries: **Cobra** (CLI), **BurntSushi/toml** (config), **hashicorp/mdns** (LAN discovery in `setup`), **golang.org/x/sys** (Unix `flock` for a single running `watch`). Everything else uses the Go standard library.
