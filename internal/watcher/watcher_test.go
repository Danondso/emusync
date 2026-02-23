//go:build linux

package watcher

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/dublin/emusync/internal/model"
)

func TestWatcher_DetectsLaunch(t *testing.T) {
	emulators := []model.EmulatorConfig{
		{
			Name:         "test-sleep",
			ProcessNames: []string{"sleep"},
		},
	}

	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep process: %v", err)
	}
	defer cmd.Process.Kill()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWatcher(emulators, 50*time.Millisecond)

	go func() {
		_ = w.Run(ctx)
	}()

	select {
	case evt := <-w.Events():
		if evt.EventType != Launched {
			t.Errorf("expected Launched event, got %v", evt.EventType)
		}
		if evt.Binary != "sleep" {
			t.Errorf("expected binary \"sleep\", got %q", evt.Binary)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for Launched event")
	}

	cmd.Process.Kill()
	cancel()
}

func TestWatcher_DetectsExit(t *testing.T) {
	emulators := []model.EmulatorConfig{
		{
			Name:         "test-sleep",
			ProcessNames: []string{"sleep"},
		},
	}

	cmd := exec.Command("sleep", "10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start sleep process: %v", err)
	}
	targetPID := cmd.Process.Pid

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := NewWatcher(emulators, 50*time.Millisecond)

	go func() {
		_ = w.Run(ctx)
	}()

	// Wait for Launched event for our specific PID (drain others).
	deadline := time.After(5 * time.Second)
	for {
		select {
		case evt := <-w.Events():
			if evt.EventType == Launched && evt.PID == targetPID {
				goto launched
			}
		case <-deadline:
			t.Fatal("timed out waiting for Launched event for target PID")
		}
	}
launched:

	// Kill the process to trigger an Exited event.
	cmd.Process.Kill()
	cmd.Wait()

	deadline = time.After(5 * time.Second)
	for {
		select {
		case evt := <-w.Events():
			if evt.EventType == Exited && evt.PID == targetPID {
				goto done
			}
		case <-deadline:
			t.Fatal("timed out waiting for Exited event for target PID")
		}
	}
done:
	cancel()
}

func TestWatcher_CancelStopsLoop(t *testing.T) {
	emulators := []model.EmulatorConfig{
		{
			Name:         "test-sleep",
			ProcessNames: []string{"sleep"},
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	w := NewWatcher(emulators, 50*time.Millisecond)

	err := w.Run(ctx)
	if err != context.Canceled {
		t.Errorf("Run() returned %v, want context.Canceled", err)
	}
}
