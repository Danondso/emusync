//go:build unix

package watchlock

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// Acquire tries to create/open path and take a non-blocking exclusive flock.
// Call release when done to close the file descriptor (and drop the lock).
func Acquire(path string) (release func(), err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("watch lock dir: %w", err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0600)
	if err != nil {
		return nil, fmt.Errorf("watch lock file: %w", err)
	}

	if err := unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return nil, fmt.Errorf("%w (%s)", ErrAlreadyHeld, path)
		}
		return nil, fmt.Errorf("watch lock flock: %w", err)
	}

	var once sync.Once
	release = func() {
		once.Do(func() {
			_ = f.Close()
		})
	}
	return release, nil
}
