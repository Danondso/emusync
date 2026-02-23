package watcher

import (
	"context"
	"log/slog"
	"time"

	"github.com/dublin/emusync/internal/model"
)

// EventType represents a process lifecycle event.
type EventType int

const (
	Launched EventType = iota
	Exited
)

// ProcessEvent is emitted when an emulator process starts or stops.
type ProcessEvent struct {
	Emulator  *model.EmulatorConfig
	EventType EventType
	PID       int
	Binary    string
}

// trackedProcess records a running emulator process.
type trackedProcess struct {
	emulator *model.EmulatorConfig
	binary   string
}

// Watcher monitors /proc for emulator process lifecycle events.
type Watcher struct {
	emulators    []model.EmulatorConfig
	pollInterval time.Duration
	tracked      map[int]trackedProcess // PID -> tracked info
	events       chan ProcessEvent
}

// NewWatcher creates a new process watcher.
func NewWatcher(emulators []model.EmulatorConfig, pollInterval time.Duration) *Watcher {
	return &Watcher{
		emulators:    emulators,
		pollInterval: pollInterval,
		tracked:      make(map[int]trackedProcess),
		events:       make(chan ProcessEvent, 32),
	}
}

// Events returns the channel of process events.
func (w *Watcher) Events() <-chan ProcessEvent {
	return w.events
}

// Run starts the watcher loop. It blocks until ctx is cancelled.
func (w *Watcher) Run(ctx context.Context) error {
	slog.Info("watcher started",
		"emulators", len(w.emulators),
		"poll_interval", w.pollInterval,
	)

	ticker := time.NewTicker(w.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			close(w.events)
			return ctx.Err()
		case <-ticker.C:
			w.poll()
		}
	}
}

func (w *Watcher) poll() {
	processes, err := ListProcesses()
	if err != nil {
		slog.Error("listing processes", "error", err)
		return
	}

	// Build set of currently running PIDs that match our emulators
	currentPIDs := make(map[int]trackedProcess)

	for _, proc := range processes {
		names := w.resolveNames(&proc)
		for _, emu := range w.emulators {
			for _, name := range names {
				if emu.MatchesProcess(name) {
					currentPIDs[proc.PID] = trackedProcess{
						emulator: &emu,
						binary:   name,
					}
					break
				}
			}
			if _, found := currentPIDs[proc.PID]; found {
				break
			}
		}
	}

	// Detect newly launched processes
	for pid, tp := range currentPIDs {
		if _, wasTracked := w.tracked[pid]; !wasTracked {
			slog.Info("emulator launched",
				"emulator", tp.emulator.Name,
				"pid", pid,
				"binary", tp.binary,
			)
			select {
			case w.events <- ProcessEvent{
				Emulator:  tp.emulator,
				EventType: Launched,
				PID:       pid,
				Binary:    tp.binary,
			}:
			default:
				slog.Warn("event channel full, dropping launch event", "emulator", tp.emulator.Name)
			}
		}
	}

	// Detect exited processes
	for pid, tp := range w.tracked {
		if _, stillRunning := currentPIDs[pid]; !stillRunning {
			slog.Info("emulator exited",
				"emulator", tp.emulator.Name,
				"pid", pid,
				"binary", tp.binary,
			)
			select {
			case w.events <- ProcessEvent{
				Emulator:  tp.emulator,
				EventType: Exited,
				PID:       pid,
				Binary:    tp.binary,
			}:
			default:
				slog.Warn("event channel full, dropping exit event", "emulator", tp.emulator.Name)
			}
		}
	}

	// Update tracked set
	w.tracked = currentPIDs
}

// resolveNames returns all possible binary names for a process,
// handling Flatpak and Proton wrappers.
func (w *Watcher) resolveNames(proc *ProcessInfo) []string {
	// Check for Flatpak/bwrap wrapper
	if IsFlatpakProcess(proc) {
		names := FlatpakNames(proc)
		if len(names) > 0 {
			return names
		}
	}

	// Check for Proton/Wine wrapper
	if IsProtonProcess(proc) {
		names := ProtonNames(proc)
		if len(names) > 0 {
			return names
		}
	}

	// Normal process
	return proc.AllNames()
}
