package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// Setup initializes the global slog logger to write to both stderr and a log file.
// It returns a cleanup function that closes the log file.
func Setup(logPath string, verbose bool) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return nil, err
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return nil, err
	}

	w := io.MultiWriter(os.Stderr, logFile)

	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	handler := slog.NewTextHandler(w, &slog.HandlerOptions{Level: level})
	slog.SetDefault(slog.New(handler))
	return func() { logFile.Close() }, nil
}
