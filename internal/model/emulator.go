package model

import (
	"path/filepath"
	"strings"
)

// EmulatorConfig maps emulator process names to save directories.
type EmulatorConfig struct {
	Name         string   `toml:"name" json:"name"`
	ProcessNames []string `toml:"process_names" json:"process_names"`
	SavePaths    []string `toml:"save_paths" json:"save_paths"`
}

// MatchesProcess returns true if processName matches any configured process name.
// Handles exact match, case-insensitive match, and .AppImage suffix stripping.
func (e *EmulatorConfig) MatchesProcess(processName string) bool {
	// Strip path to get just the binary name
	base := filepath.Base(processName)

	// Also prepare a version with .AppImage stripped
	stripped := strings.TrimSuffix(base, ".AppImage")

	for _, name := range e.ProcessNames {
		configBase := filepath.Base(name)
		configStripped := strings.TrimSuffix(configBase, ".AppImage")

		// Exact match
		if base == configBase {
			return true
		}
		// Case-insensitive match
		if strings.EqualFold(base, configBase) {
			return true
		}
		// Match with .AppImage stripped from either side
		if stripped == configStripped || strings.EqualFold(stripped, configStripped) {
			return true
		}
		// Match .exe suffix for Proton games
		if strings.HasSuffix(strings.ToLower(base), ".exe") && strings.EqualFold(base, configBase) {
			return true
		}
	}
	return false
}
