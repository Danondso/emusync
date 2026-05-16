package watchlock

import (
	"errors"
	"path/filepath"

	"github.com/dublin/emusync/internal/config"
)

// ErrAlreadyHeld indicates another process holds an exclusive flock on the watch lock file.
var ErrAlreadyHeld = errors.New("emusync watch is already running")

// DefaultPath returns the lock file path next to client state (~/.local/share/emusync/watch.lock).
func DefaultPath() (string, error) {
	statePath, err := config.DefaultStatePath()
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(statePath), "watch.lock"), nil
}
