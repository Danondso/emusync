package watcher

import (
	"path/filepath"
	"strings"
)

// IsFlatpakProcess checks if a process is running inside a Flatpak/bwrap sandbox.
func IsFlatpakProcess(proc *ProcessInfo) bool {
	if proc.Comm == "bwrap" {
		return true
	}
	if len(proc.Cmdline) > 0 {
		base := filepath.Base(proc.Cmdline[0])
		if base == "bwrap" || base == "flatpak" {
			return true
		}
	}
	return false
}

// ExtractFlatpakBinary extracts the actual emulator binary name from a
// Flatpak/bwrap process cmdline. bwrap puts the real binary after "--" in its args.
func ExtractFlatpakBinary(proc *ProcessInfo) string {
	if len(proc.Cmdline) == 0 {
		return ""
	}

	// Look for the binary after "--" separator
	afterSep := false
	for _, arg := range proc.Cmdline {
		if arg == "--" {
			afterSep = true
			continue
		}
		if afterSep {
			// First arg after -- is the actual binary
			return filepath.Base(arg)
		}
	}

	// Fallback: scan all args for something that looks like a binary (not a flag)
	for i := len(proc.Cmdline) - 1; i >= 1; i-- {
		arg := proc.Cmdline[i]
		if strings.HasPrefix(arg, "-") || strings.HasPrefix(arg, "/") {
			continue
		}
		// Could be the binary name
		return filepath.Base(arg)
	}

	return ""
}

// FlatpakNames returns all possible binary names from a Flatpak process.
func FlatpakNames(proc *ProcessInfo) []string {
	var names []string
	if name := ExtractFlatpakBinary(proc); name != "" {
		names = append(names, name)
	}
	// Also include child processes' comm names that might be the actual emulator
	return names
}
