package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetup_CreatesLogFile(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "logs", "test.log")

	cleanup, err := Setup(logPath, false)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer cleanup()

	// Verify the file was created
	if _, err := os.Stat(logPath); os.IsNotExist(err) {
		t.Fatal("log file was not created")
	}

	// Write a log message
	slog.Info("hello from test")

	// Verify file is non-empty
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if len(data) == 0 {
		t.Error("log file is empty after writing a log message")
	}
	if !strings.Contains(string(data), "hello from test") {
		t.Errorf("log file does not contain expected message, got: %s", string(data))
	}
}

func TestSetup_VerboseEnablesDebug(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "verbose.log")

	cleanup, err := Setup(logPath, true)
	if err != nil {
		t.Fatalf("Setup failed: %v", err)
	}
	defer cleanup()

	// Write a debug-level message
	slog.Debug("test-debug-msg")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log file: %v", err)
	}
	if !strings.Contains(string(data), "test-debug-msg") {
		t.Errorf("debug message not found in log file when verbose=true, got: %s", string(data))
	}
}
