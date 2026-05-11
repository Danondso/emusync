# emusync

Self-hosted emulator save file synchronization for Linux desktops and Steam Decks.

Emusync automatically syncs your emulator saves and savestates across devices -- pulling the latest saves when an emulator launches and uploading them when it closes. A lightweight server stores versioned saves with conflict detection, and a client agent monitors emulator processes in the background.

## Features

- **Automatic sync** -- saves upload on emulator exit, download on launch
- **Conflict detection** -- hash-based conflict detection with interactive resolution
- **Versioned backups** -- server retains configurable history of each save file
- **Process monitoring** -- detects emulators via `/proc`, including Flatpak, Proton/Wine, and AppImage wrappers
- **19 emulators preconfigured** -- RetroArch, Dolphin, PCSX2, RPCS3, Ryujinx, DuckStation, PPSSPP, Azahar/Citra, Cemu, mGBA, melonDS, MAME, Flycast, ScummVM, PrimeHack, RMG, Vita3K, Xenia, shadPS4
- **Single binary** -- static Go executable, no runtime dependencies

## Quick Start

### Server

1. Copy the example environment file and set an auth token:

   ```bash
   cp deploy/docker/.env.example .env
   # Edit .env and set EMUSYNC_AUTH_TOKEN to a secure value
   ```

2. Start the server:

   ```bash
   make docker-up
   ```

   The server listens on port 8080 with save data persisted in a Docker volume.

### Client

1. Build and install:

   ```bash
   make install
   ```

2. Generate a default config:

   ```bash
   emusync init
   ```

3. Edit `~/.config/emusync/config.toml` -- set your server host, auth token, device ID, and saves path.

4. Start the watcher:

   ```bash
   emusync watch
   ```

   Or install as a systemd user service for automatic startup:

   ```bash
   make install-service
   systemctl --user enable --now emusync-watch
   ```

## Usage

```
emusync watch              # Monitor emulators and auto-sync saves
emusync push               # Manually upload all saves
emusync pull               # Manually download all saves
emusync status             # Show local vs server sync state
emusync history <emu> <path>  # View version history for a save file
emusync resolve            # Interactively resolve sync conflicts
emusync server             # Run the server (used by Docker entrypoint)
```

## Configuration

Config file: `~/.config/emusync/config.toml`

```toml
[server]
host = "192.168.1.100"
port = 8080
auth_token = "changeme-generate-a-real-token"

[client]
device_id = "my-device"
saves_path = "~/Emulation/saves"
backup_path = "~/Emulation/saves/.sync-backups"
max_local_backups = 10

[sync]
auto_sync_on_close = true
auto_sync_on_launch = true
conflict_strategy = "prompt"   # "prompt", "newest", or "keep-both"
poll_interval_ms = 2000
post_exit_delay_ms = 2000
```

### Adding Custom Emulators

Add an `[[emulators]]` block to your config. Save paths are relative to `saves_path` unless absolute:

```toml
[[emulators]]
name = "my-emulator"
process_names = ["my-emu", "my-emu.AppImage"]
save_paths = ["my-emulator/saves", "my-emulator/states"]
```

### Server Environment Variables

| Variable | Default | Description |
|---|---|---|
| `EMUSYNC_PORT` | `8080` | Listen port |
| `EMUSYNC_DATA_DIR` | `/data` | Save data directory |
| `EMUSYNC_AUTH_TOKEN` | — | Bearer token for API auth |
| `EMUSYNC_MAX_BACKUPS` | `10` | Versions to retain per file |

## Building

Install [Go](https://go.dev/dl/) **1.26 or newer** (see the `go` directive in `go.mod`; CI uses that version).

```bash
make build      # Build binary
make test       # Unit tests (excludes E2E tag)
make test-e2e   # Full stack E2E via Docker Compose — requires Docker
make docker     # Build Docker image (`docker compose build`)
```

Pull requests run the same checks in [GitHub Actions](.github/workflows/ci.yml) (`go vet`, `staticcheck`, tests, then E2E).

## License

[MIT](LICENSE)
