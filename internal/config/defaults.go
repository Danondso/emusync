package config

import (
	"github.com/dublin/emusync/internal/model"
)

// DefaultEmulators returns the default emulator mappings.
func DefaultEmulators() []model.EmulatorConfig {
	return []model.EmulatorConfig{
		{
			Name:         "retroarch",
			ProcessNames: []string{"retroarch"},
			SavePaths:    []string{"retroarch/saves", "retroarch/states"},
		},
		{
			Name:         "dolphin",
			ProcessNames: []string{"dolphin-emu"},
			SavePaths:    []string{"dolphin/GC", "dolphin/Wii", "dolphin/StateSaves"},
		},
		{
			Name:         "pcsx2",
			ProcessNames: []string{"pcsx2-qt", "pcsx2-Qt.AppImage"},
			SavePaths:    []string{"pcsx2/saves", "pcsx2/states"},
		},
		{
			Name:         "rpcs3",
			ProcessNames: []string{"rpcs3", "rpcs3.AppImage"},
			SavePaths:    []string{"rpcs3/saves", "rpcs3/trophy"},
		},
		{
			Name:         "ryujinx",
			ProcessNames: []string{"ryujinx", "Ryujinx", "Ryujinx.Ava"},
			SavePaths:    []string{"ryujinx/saves", "ryujinx/system", "ryujinx/system_saves", "ryujinx/saveMeta"},
		},
		{
			Name:         "duckstation",
			ProcessNames: []string{"duckstation-qt", "DuckStation.AppImage"},
			SavePaths:    []string{"duckstation/saves", "duckstation/states"},
		},
		{
			Name:         "ppsspp",
			ProcessNames: []string{"PPSSPPSDL", "ppsspp"},
			SavePaths:    []string{"ppsspp/saves", "ppsspp/states"},
		},
		{
			Name:         "azahar",
			ProcessNames: []string{"azahar", "azahar.AppImage", "citra-qt"},
			SavePaths:    []string{"azahar/saves", "azahar/states", "citra/saves", "citra/states"},
		},
		{
			Name:         "cemu",
			ProcessNames: []string{"Cemu", "Cemu.AppImage"},
			SavePaths:    []string{"Cemu/saves"},
		},
		{
			Name:         "mgba",
			ProcessNames: []string{"mgba-qt", "mGBA.AppImage"},
			SavePaths:    []string{"mgba/saves", "mgba/states"},
		},
		{
			Name:         "melonds",
			ProcessNames: []string{"melonDS"},
			SavePaths:    []string{"melonds/saves", "melonds/states"},
		},
		{
			Name:         "mame",
			ProcessNames: []string{"mame"},
			SavePaths:    []string{"MAME/saves", "MAME/states"},
		},
		{
			Name:         "flycast",
			ProcessNames: []string{"flycast"},
			SavePaths:    []string{"flycast/saves", "flycast/states"},
		},
		{
			Name:         "scummvm",
			ProcessNames: []string{"scummvm"},
			SavePaths:    []string{"scummvm/saves"},
		},
		{
			Name:         "primehack",
			ProcessNames: []string{"primehack"},
			SavePaths:    []string{"primehack/GC", "primehack/Wii", "primehack/StateSaves"},
		},
		{
			Name:         "rmg",
			ProcessNames: []string{"RMG"},
			SavePaths:    []string{"RMG/saves", "RMG/states"},
		},
		{
			Name:         "vita3k",
			ProcessNames: []string{"Vita3K"},
			SavePaths:    []string{"Vita3K/saves"},
		},
		{
			Name:         "xenia",
			ProcessNames: []string{"xenia", "xenia_canary.exe"},
			SavePaths:    []string{"xenia/saves"},
		},
		{
			Name:         "shadps4",
			ProcessNames: []string{"shadps4", "Shadps4-qt.AppImage"},
			SavePaths:    []string{"shadps4/saves"},
		},
	}
}

// DefaultConfigContent returns the full default config.toml content.
func DefaultConfigContent() string {
	return `[server]
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
conflict_strategy = "prompt"
poll_interval_ms = 2000
post_exit_delay_ms = 2000

# --- Emulator Definitions ---
# Each [[emulators]] block maps process names to save directories.
# Paths are relative to saves_path unless they start with "/".

[[emulators]]
name = "retroarch"
process_names = ["retroarch"]
save_paths = ["retroarch/saves", "retroarch/states"]

[[emulators]]
name = "dolphin"
process_names = ["dolphin-emu"]
save_paths = ["dolphin/GC", "dolphin/Wii", "dolphin/StateSaves"]

[[emulators]]
name = "pcsx2"
process_names = ["pcsx2-qt", "pcsx2-Qt.AppImage"]
save_paths = ["pcsx2/saves", "pcsx2/states"]

[[emulators]]
name = "rpcs3"
process_names = ["rpcs3", "rpcs3.AppImage"]
save_paths = ["rpcs3/saves", "rpcs3/trophy"]

[[emulators]]
name = "ryujinx"
process_names = ["ryujinx", "Ryujinx", "Ryujinx.Ava"]
save_paths = ["ryujinx/saves", "ryujinx/system", "ryujinx/system_saves", "ryujinx/saveMeta"]

[[emulators]]
name = "duckstation"
process_names = ["duckstation-qt", "DuckStation.AppImage"]
save_paths = ["duckstation/saves", "duckstation/states"]

[[emulators]]
name = "ppsspp"
process_names = ["PPSSPPSDL", "ppsspp"]
save_paths = ["ppsspp/saves", "ppsspp/states"]

[[emulators]]
name = "azahar"
process_names = ["azahar", "azahar.AppImage", "citra-qt"]
save_paths = ["azahar/saves", "azahar/states", "citra/saves", "citra/states"]

[[emulators]]
name = "cemu"
process_names = ["Cemu", "Cemu.AppImage"]
save_paths = ["Cemu/saves"]

[[emulators]]
name = "mgba"
process_names = ["mgba-qt", "mGBA.AppImage"]
save_paths = ["mgba/saves", "mgba/states"]

[[emulators]]
name = "melonds"
process_names = ["melonDS"]
save_paths = ["melonds/saves", "melonds/states"]

[[emulators]]
name = "mame"
process_names = ["mame"]
save_paths = ["MAME/saves", "MAME/states"]

[[emulators]]
name = "flycast"
process_names = ["flycast"]
save_paths = ["flycast/saves", "flycast/states"]

[[emulators]]
name = "scummvm"
process_names = ["scummvm"]
save_paths = ["scummvm/saves"]

[[emulators]]
name = "primehack"
process_names = ["primehack"]
save_paths = ["primehack/GC", "primehack/Wii", "primehack/StateSaves"]

[[emulators]]
name = "rmg"
process_names = ["RMG"]
save_paths = ["RMG/saves", "RMG/states"]

[[emulators]]
name = "vita3k"
process_names = ["Vita3K"]
save_paths = ["Vita3K/saves"]

[[emulators]]
name = "xenia"
process_names = ["xenia", "xenia_canary.exe"]
save_paths = ["xenia/saves"]

[[emulators]]
name = "shadps4"
process_names = ["shadps4", "Shadps4-qt.AppImage"]
save_paths = ["shadps4/saves"]
`
}
