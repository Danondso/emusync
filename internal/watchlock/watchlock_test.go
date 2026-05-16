//go:build linux

package watchlock

import (
	"errors"
	"path/filepath"
	"testing"
)

func TestAcquire_releaseAcquireAgain(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "watch.lock")

	release1, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	release1()

	release2, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	release2()
}

func TestAcquire_conflict(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "watch.lock")

	release1, err := Acquire(path)
	if err != nil {
		t.Fatal(err)
	}
	defer release1()

	_, err = Acquire(path)
	if err == nil {
		t.Fatal("expected second Acquire to fail")
	}
	if !errors.Is(err, ErrAlreadyHeld) {
		t.Fatalf("want ErrAlreadyHeld in chain, got: %v", err)
	}
}
