package watcher

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ProcessInfo holds information about a running process.
type ProcessInfo struct {
	PID     int
	Cmdline []string // argv from /proc/<pid>/cmdline
	Comm    string   // from /proc/<pid>/comm
	Exe     string   // readlink of /proc/<pid>/exe
}

// BinaryName returns the most likely binary name for matching purposes.
// It checks exe symlink first, then argv[0], then comm.
func (p *ProcessInfo) BinaryName() string {
	if p.Exe != "" {
		return filepath.Base(p.Exe)
	}
	if len(p.Cmdline) > 0 && p.Cmdline[0] != "" {
		return filepath.Base(p.Cmdline[0])
	}
	return p.Comm
}

// AllNames returns all possible names this process might be matched against.
// This includes the binary name, comm, and any interesting names from cmdline.
func (p *ProcessInfo) AllNames() []string {
	seen := make(map[string]bool)
	var names []string
	add := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}

	add(p.BinaryName())
	add(p.Comm)

	// Add argv[0] base name
	if len(p.Cmdline) > 0 {
		add(filepath.Base(p.Cmdline[0]))
	}

	return names
}

// ListProcesses returns info about all running processes.
func ListProcesses() ([]ProcessInfo, error) {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil, fmt.Errorf("reading /proc: %w", err)
	}

	var processes []ProcessInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid, err := strconv.Atoi(entry.Name())
		if err != nil {
			continue // Not a PID directory
		}

		info := readProcessInfo(pid)
		if info != nil {
			processes = append(processes, *info)
		}
	}
	return processes, nil
}

func readProcessInfo(pid int) *ProcessInfo {
	pidDir := fmt.Sprintf("/proc/%d", pid)

	info := &ProcessInfo{PID: pid}

	// Read cmdline (null-delimited)
	cmdlineBytes, err := os.ReadFile(filepath.Join(pidDir, "cmdline"))
	if err != nil {
		return nil
	}
	if len(cmdlineBytes) > 0 {
		// Split on null bytes, trim trailing null
		cmdlineBytes = bytes.TrimRight(cmdlineBytes, "\x00")
		parts := bytes.Split(cmdlineBytes, []byte{0})
		for _, p := range parts {
			info.Cmdline = append(info.Cmdline, string(p))
		}
	}

	// Read comm
	commBytes, err := os.ReadFile(filepath.Join(pidDir, "comm"))
	if err == nil {
		info.Comm = strings.TrimSpace(string(commBytes))
	}

	// Read exe symlink
	exe, err := os.Readlink(filepath.Join(pidDir, "exe"))
	if err == nil {
		info.Exe = exe
	}

	return info
}
