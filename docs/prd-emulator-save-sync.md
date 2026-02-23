# PRD: Emulator Save Sync (emusync)

## 1. Introduction/Overview

Emulator users who play across multiple devices (desktop PCs, Steam Decks, laptops) currently have no reliable, emulator-aware way to keep their save files in sync. Generic file sync tools like Synology Drive produce conflicts and have no understanding of emulator save structures. This project builds a self-hosted, Docker-based save sync service that automatically uploads saves when an emulator closes and syncs them to other clients on demand, enabling seamless cross-device play.

The system is **emulator-manager agnostic** -- it works with any emulator setup (EmuDeck, manual installs, RetroDECK, etc.) by monitoring emulator processes directly and syncing configured save directories. No dependency on any specific emulator management tool.

The system consists of two components:
- **Server:** A Docker container hosting a sync daemon that stores save files with versioning and conflict metadata.
- **Client:** A lightweight agent that runs on each device, monitors emulator processes, and handles upload/download/conflict resolution.

## 2. Goals

- Automatically sync emulator save files (saves and savestates) across Linux desktops and Steam Decks after an emulator session ends.
- Detect and surface conflicts when saves have diverged, prompting the user to choose while keeping backups of both versions.
- Run entirely self-hosted in a Docker container with no dependency on any specific NAS vendor, cloud provider, or emulator management tool.
- Work with any emulator setup by monitoring emulator processes directly -- not tied to EmuDeck, RetroDECK, or any specific frontend.
- Support configurable save directory mappings so users can point emusync at whatever directory structure they use.
- Maintain a versioned backup history of all saves so no data is ever permanently lost.

## 3. User Stories

- **US-1:** As a player, I want my save files to automatically upload to the server when I close an emulator, so I don't have to think about syncing.
- **US-2:** As a player, I want to see my latest saves when I start playing on a different device, so I can pick up where I left off.
- **US-3:** As a player, when a save conflict exists (e.g., I played the same game on two devices without syncing), I want to be prompted to choose which save to keep, with both versions backed up.
- **US-4:** As a player, I want savestates synced alongside regular saves, so I can resume from quick saves on any device.
- **US-5:** As a player, I want to be able to restore a previous version of a save if something goes wrong.
- **US-6:** As a system administrator, I want to run the server as a Docker container on any Linux host without vendor lock-in.

## 4. Functional Requirements

### 4.1 Server

1. The server must run as a Docker container with a single `docker-compose.yml` for deployment.
2. The server must expose an rsync daemon (over SSH) or a simple REST API for receiving and serving save files.
3. The server must store saves organized by: `/{device-id}/{emulator}/{save-type}/` mirroring the EmuDeck saves directory structure.
4. The server must maintain a canonical "latest" copy of each save file plus a versioned backup history (minimum 10 previous versions per file).
5. The server must store metadata for each save: SHA-256 hash, timestamp, source device ID, and file size.
6. The server must detect when an incoming save conflicts with the current latest (i.e., the client's base version doesn't match the server's latest) and flag it for conflict resolution.
7. The server must persist all data to a Docker volume for durability.
8. The server must support configuration via environment variables (port, storage path, max backup versions, auth token).

### 4.2 Client

9. The client must run a standalone process watcher (polling `/proc` or using `netlink` process events) to detect when monitored emulator processes exit. It must not depend on any emulator manager's launch scripts. The list of monitored process names must be user-configurable.
10. The client must identify which emulator just closed and map it to the correct save directory based on the user's configured process-to-directory mappings.
11. The client must compute SHA-256 hashes of local save files and compare them against the server's latest hashes.
12. If local saves have changed since last sync, the client must upload the changed files to the server.
13. If the server has newer saves (from another device), the client must download them before the next play session.
14. The client must run a pre-launch sync check: before an emulator starts, pull the latest saves from the server to ensure the player has the most recent data.
15. The client must handle conflicts by presenting a notification/prompt to the user with:
    - Timestamp and source device of each version
    - File size of each version
    - Options: "Keep Local", "Keep Remote", "Keep Both (rename)"
16. Regardless of conflict resolution choice, the client must back up the non-chosen version locally under `~/Emulation/saves/.sync-backups/{emulator}/{timestamp}/`.
17. The client must be installable as a systemd user service that starts on login.
18. The client must support a CLI for manual operations: `emusync status`, `emusync pull`, `emusync push`, `emusync history {emulator}`, `emusync resolve`.
19. The client must log all sync operations to `~/.local/share/emusync/sync.log`.

### 4.3 Emulator Configuration

20. The system must use a user-defined configuration that maps emulator process names to their save directories. The config must support multiple save paths per emulator (e.g., saves and states in separate directories). A default config ships with common EmuDeck-style mappings as a starting point, but all paths are user-editable.

21. The default emulator mappings (used as a template/starting point) are:

    | Emulator | Process Name(s) | Default Save Paths | Contents |
    |----------|----------------|-------------------|----------|
    | RetroArch | `retroarch` | `retroarch/saves/`, `retroarch/states/` | Saves and savestates |
    | Dolphin | `dolphin-emu` | `dolphin/GC/`, `dolphin/Wii/`, `dolphin/StateSaves/` | Per-console saves + states |
    | PCSX2 | `pcsx2-qt` | `pcsx2/saves/`, `pcsx2/states/` | Memory cards + states |
    | RPCS3 | `rpcs3` | `rpcs3/saves/`, `rpcs3/trophy/` | Game saves + trophy data |
    | Ryujinx | `ryujinx`, `Ryujinx` | `ryujinx/saves/`, `ryujinx/system/`, `ryujinx/system_saves/`, `ryujinx/saveMeta/` | Full save tree |
    | DuckStation | `duckstation-qt` | `duckstation/saves/`, `duckstation/states/` | Memory cards + states |
    | PPSSPP | `PPSSPPSDL`, `ppsspp` | `ppsspp/saves/`, `ppsspp/states/` | PSP saves + states |
    | Azahar/Citra | `azahar`, `citra-qt` | `azahar/saves/`, `azahar/states/`, `citra/saves/`, `citra/states/` | 3DS saves + states |
    | Cemu | `Cemu` | `Cemu/saves/` | Wii U saves |
    | mGBA | `mgba-qt` | `mgba/saves/`, `mgba/states/` | GBA saves + states |
    | melonDS | `melonDS` | `melonds/saves/`, `melonds/states/` | DS saves + states |
    | MAME | `mame` | `MAME/saves/`, `MAME/states/` | Arcade saves + states |
    | Flycast | `flycast` | `flycast/saves/`, `flycast/states/` | Dreamcast saves + states |
    | ScummVM | `scummvm` | `scummvm/saves/` | Point-and-click saves |
    | PrimeHack | `primehack` | `primehack/GC/`, `primehack/Wii/`, `primehack/StateSaves/` | Same structure as Dolphin |
    | RMG | `RMG` | `RMG/saves/`, `RMG/states/` | N64 saves + states |
    | Vita3K | `Vita3K` | `Vita3K/saves/` | Vita saves |
    | Xenia | `xenia` | `xenia/saves/` | Xbox 360 saves |
    | shadPS4 | `shadps4` | `shadps4/saves/` | PS4 saves |

22. Users must be able to add custom emulator entries (process name + save paths) via the config file for any emulator not in the defaults.
23. The system must support a configurable base saves path (default: `~/Emulation/saves`) with all emulator paths resolved relative to it. Users with non-standard layouts can override paths to be absolute.
24. The system must support auto-discovery mode as an option: scan the base saves directory for subdirectories and offer to add unrecognized ones to the config.

### 4.4 Transport

23. The primary sync transport must be rsync over SSH for efficiency (delta transfers, compression).
24. The client must authenticate to the server using SSH key-based auth (no passwords).
25. The system must support configuring the server address and port via a config file at `~/.config/emusync/config.toml`.

## 5. Non-Goals (Out of Scope)

- Syncing ROM files, BIOS files, or texture packs between devices.
- Supporting Windows or macOS clients (Linux and Steam Deck only for v1).
- Providing a web UI for the server (CLI and notifications only for v1).
- Automatic emulator installation or configuration.
- Real-time sync while an emulator is running (sync only triggers on emulator close/open).
- Integration with commercial cloud storage providers (Google Drive, Dropbox, etc.).
- Syncing ES-DE gamelists or frontend metadata (too conflict-prone and not save data).
- Dependency on any specific emulator management tool (EmuDeck, RetroDECK, etc.).

## 6. Design Considerations

### Conflict Resolution UI
- On desktop Linux: Use `notify-send` for initial notification with a follow-up terminal prompt or a simple GTK dialog (via `zenity` or `kdialog`).
- On Steam Deck (Gaming Mode): Integrate with Steam Deck's notification system via the CEF-based Steam overlay. Steam exposes a notification interface through its `steam://` protocol and the `steamclient.so` library. The client should:
  1. Send a Steam notification when a conflict is detected (visible in Gaming Mode).
  2. Provide a Non-Steam Game shortcut or a Steam Deck plugin (Decky Loader compatible) for resolving conflicts from Gaming Mode.
  3. As a fallback if Steam notifications are unavailable, auto-resolve with "newest wins" + backup, and queue a full conflict review for the next `emusync resolve` CLI invocation or Desktop Mode session.

### Directory Layout (Server)
```
/data/
  canonical/           # Latest version of each save
    retroarch/
      saves/
      states/
    dolphin/
      GC/
      ...
  backups/             # Versioned history
    retroarch/
      saves/
        {filename}.{timestamp}.bak
  metadata/            # JSON metadata per file
    retroarch/
      saves/
        {filename}.json
```

### Client Config Example (`~/.config/emusync/config.toml`)
```toml
[server]
host = "192.168.1.100"
port = 2222
user = "emusync"

[client]
device_id = "desktop-pop-os"
saves_path = "~/Emulation/saves"
backup_path = "~/Emulation/saves/.sync-backups"
max_local_backups = 10

[sync]
auto_sync_on_close = true
auto_sync_on_launch = true
conflict_strategy = "prompt"  # "prompt", "newest", "keep-both"

# Process watcher settings
poll_interval_ms = 2000  # How often to check for emulator process exits

# Emulator definitions - add/override as needed
[[emulators]]
name = "retroarch"
process_names = ["retroarch"]
save_paths = ["retroarch/saves", "retroarch/states"]

[[emulators]]
name = "dolphin"
process_names = ["dolphin-emu"]
save_paths = ["dolphin/GC", "dolphin/Wii", "dolphin/StateSaves"]

[[emulators]]
name = "rpcs3"
process_names = ["rpcs3"]
save_paths = ["rpcs3/saves", "rpcs3/trophy"]

# Custom emulator example - any emulator can be added
[[emulators]]
name = "my-custom-emu"
process_names = ["custom-emu", "custom-emu-qt"]
save_paths = ["/absolute/path/to/saves"]
```

## 7. Technical Considerations

- **Process Monitoring:** The client runs a standalone process watcher as a systemd user service. It polls `/proc` at a configurable interval (default 2s) to detect when monitored emulator processes exit. This approach is emulator-manager agnostic -- it doesn't depend on EmuDeck, RetroDECK, or any specific launcher scripts. Alternatively, Linux `netlink` process connector (`PROC_EVENT_EXIT`) can be used for event-driven detection without polling, but requires `CAP_NET_ADMIN` or root.
- **Delta Sync:** rsync's delta transfer algorithm means only changed bytes are transmitted, which is important for large savestates (some can be 50MB+).
- **Atomic Writes:** Save files should be written atomically (write to temp, then rename) to avoid corruption if sync and emulator write collide.
- **Steam Deck Considerations:** The Steam Deck runs SteamOS (Arch-based). The client should be packaged as a single static binary or installed via a simple shell script. systemd user services work on Steam Deck in Desktop Mode. For Gaming Mode notifications, investigate Decky Loader plugin integration and the Steam CEF overlay notification API.
- **Existing Synology Conflicts:** The system should handle existing `_Conflict` directories in the saves folder gracefully (ignore them or offer to clean them up via `emusync cleanup`).
- **Dependencies:** Minimal - rsync, openssh-client, and optionally zenity/kdialog for desktop GUI prompts. All commonly available on Linux.
- **Language:** Go or Rust for a single static binary with no runtime dependencies. Single binary simplifies distribution across desktop and Steam Deck. Server can be the same binary with a `server` subcommand, or a separate lightweight Go service.

## 8. Success Metrics

- A user can close a game on Device A, walk to Device B, launch the same game, and have their latest save available within 60 seconds.
- Zero save file corruption incidents caused by the sync system.
- Conflict detection rate of 100% (no silent overwrites when saves have diverged).
- All backup versions are retained and recoverable via CLI.
- Server deployment takes under 5 minutes with a single `docker-compose up -d`.

## 9. Resolved Decisions

1. **Process monitoring approach:** Standalone process watcher (poll `/proc` or netlink). No dependency on EmuDeck or any launcher scripts.
2. **Steam Deck Gaming Mode UX:** Integrate with Steam Deck notifications (CEF overlay / Decky Loader plugin). Fallback to "newest wins" + backup if notification integration is unavailable.
3. **ES-DE gamelists:** Excluded from sync (out of scope).
4. **RPCS3 trophy data:** Included in sync. Trophy directories (`rpcs3/trophy/`) are synced alongside saves.
5. **Emulator manager agnostic:** The system must not depend on EmuDeck, RetroDECK, or any specific frontend. It works by monitoring process names and syncing configured directories.

## 10. Open Questions

1. **Authentication model:** Simple shared SSH key for all devices, or per-device SSH keys? Per-device allows revoking access for a single device without rotating keys on all others.
2. **Polling interval vs. resource usage:** Default 2s poll of `/proc` is lightweight, but should we offer a netlink-based event-driven mode for users who want zero-latency detection at the cost of requiring elevated permissions?
3. **Decky Loader plugin scope:** Should the Decky Loader plugin be part of v1, or a follow-up? It adds significant complexity but provides the best Steam Deck Gaming Mode experience.
4. **Config file format:** TOML is proposed -- should we also support YAML for users who prefer it, or keep it single-format for simplicity?
5. **Initial setup wizard:** Should `emusync init` auto-detect installed emulators (by scanning for known process names in PATH and known save directories) to generate the initial config?
