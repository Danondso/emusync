package watcher

import (
	"path/filepath"
	"strings"
)

// IsProtonProcess checks if a process is running under Proton/Wine.
func IsProtonProcess(proc *ProcessInfo) bool {
	for _, arg := range proc.Cmdline {
		base := filepath.Base(arg)
		if base == "pressure-vessel-wrap" || base == "wine-preloader" ||
			base == "wine64-preloader" || base == "proton" ||
			strings.Contains(arg, "steamapps/common/Proton") {
			return true
		}
	}
	return false
}

// ExtractProtonExe extracts the .exe filename from a Proton/Wine process cmdline.
func ExtractProtonExe(proc *ProcessInfo) string {
	for _, arg := range proc.Cmdline {
		if strings.HasSuffix(strings.ToLower(arg), ".exe") {
			return filepath.Base(arg)
		}
	}
	return ""
}

// ProtonNames returns possible binary names from a Proton process.
func ProtonNames(proc *ProcessInfo) []string {
	var names []string
	if exe := ExtractProtonExe(proc); exe != "" {
		names = append(names, exe)
	}
	return names
}
