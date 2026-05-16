# emusync

Self-hosted emulator save file synchronization for Linux desktops and Steam Decks.

Emusync automatically syncs your emulator saves and savestates across devices -- pulling the latest saves when an emulator launches and uploading them when it closes. A lightweight server stores versioned saves with conflict detection, and a client watcher monitors emulator processes as a **long-running task**. On Linux desktops and Steam Deck (desktop session), run that watcher under **systemd** as a user service so it starts with your session and restarts on failure; use `emusync watch` in the foreground only when debugging.

## Features

- **Automatic sync** -- saves upload on emulator exit, download on launch
- **Conflict detection** -- hash-based conflict detection with interactive resolution
- **Versioned backups** -- server retains configurable history of each save file
- **Process monitoring** -- detects emulators via `/proc`, including Flatpak, Proton/Wine, and AppImage wrappers
- **19 emulators preconfigured** -- RetroArch, Dolphin, PCSX2, RPCS3, Ryujinx, DuckStation, PPSSPP, Azahar/Citra, Cemu, mGBA, melonDS, MAME, Flycast, ScummVM, PrimeHack, RMG, Vita3K, Xenia, shadPS4
- **Single binary** -- static Go executable, no runtime dependencies

## Quick Start

### Server

**Recommended — one-shot bootstrap** (creates a root `.env` with a random token if missing, then runs `docker compose up -d --build`):

```bash
./scripts/bootstrap-server.sh
# or: make bootstrap-server
```

- Requires Docker with the Compose v2 plugin (`docker compose`).
- Creates **`EMUSYNC_AUTH_TOKEN`** and **`EMUSYNC_ADMIN_TOKEN`** in `.env`. If `.env` already exists, tokens are left unchanged unless the admin token is missing (bootstrap appends one) or you pass **`--force-token`**, which rotates **only** the sync auth token and preserves everything else — every client must then be updated for sync.
- On failure, ensure `docker` works for your user (e.g. `docker` group).

**Manual alternative:** copy `deploy/docker/.env.example` to `.env`, set `EMUSYNC_AUTH_TOKEN`, then `make docker-up`.

The server listens on port 8080 with save data persisted in a Docker volume.

### Client

1. Build and install:

   ```bash
   make install
   ```

2. Create a starter config (optional):

   ```bash
   emusync init
   ```

3. Run interactive setup (LAN mDNS discovery, token, autodetected save roots):

   ```bash
   emusync setup
   ```

   Use `emusync setup --force` to overwrite an existing config. You can still edit `~/.config/emusync/config.toml` by hand.

   **Server LAN advertisement:** set `EMUSYNC_ADVERTISE_MDNS=true` for the server container (see `docker-compose.yml`) so clients can find it via mDNS (`_emusync._tcp`). This is best on bare-metal or host-network setups; inside default bridge Docker, mDNS usually does not reach your LAN.

4. Start the watcher (recommended: **systemd user service**):

   ```bash
   make install-service
   systemctl --user enable --now emusync-watch
   ```

   The installed unit runs **`watch --quiet`** so the informational banner stays off stdout/journal (structured logs still go to the journal and **`~/.local/share/emusync/sync.log`**).

   Foreground-only (debugging):

   ```bash
   emusync watch
   ```

## Usage

```
emusync watch [--quiet]    # Monitor emulators (--quiet for systemd/journal)
emusync setup              # Interactive config (mDNS + save path autodetect)
emusync push               # Manually upload all saves
emusync pull               # Manually download all saves
emusync status             # Show local vs server sync state
emusync history <emu> <path>  # View version history for a save file
emusync resolve            # Interactively resolve sync conflicts
emusync pull-profile       # Apply server admin profile to local [[emulators]] (needs EMUSYNC_ADMIN_TOKEN)
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

### Server admin web UI (optional)

When `EMUSYNC_ADMIN_TOKEN` is set for the server container, browse **`http://<host>:8080/admin/`**.

- Paste the same token in the page to call the admin API from your browser.
- Resolve conflicts (same JSON API as `emusync resolve`), inspect status, and edit the **profile** JSON pushed to clients via `emusync pull-profile`.

On a client:

```bash
export EMUSYNC_ADMIN_TOKEN='same-secret-as-server'
emusync pull-profile   # uses server.host from config.toml
```

### Server Environment Variables

| Variable | Default | Description |
|---|---|---|
| `EMUSYNC_PORT` | `8080` | Listen port |
| `EMUSYNC_DATA_DIR` | `/data` | Save data directory |
| `EMUSYNC_AUTH_TOKEN` | — | Bearer token for API auth |
| `EMUSYNC_ADMIN_TOKEN` | — | Bearer token for `/admin` (optional) |
| `EMUSYNC_MAX_BACKUPS` | `10` | Versions to retain per file |
| `EMUSYNC_ADVERTISE_MDNS` | `false` | Advertise `_emusync._tcp` on the LAN |

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
