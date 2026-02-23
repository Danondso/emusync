package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"
)

// ServerConfig holds server runtime configuration, typically from env vars.
type ServerConfig struct {
	Port       int
	DataDir    string
	AuthToken  string
	MaxBackups int
}

// ConfigFromEnv reads server configuration from environment variables.
func ConfigFromEnv() ServerConfig {
	cfg := ServerConfig{
		Port:       8080,
		DataDir:    "/data",
		AuthToken:  os.Getenv("EMUSYNC_AUTH_TOKEN"),
		MaxBackups: 10,
	}

	if p := os.Getenv("EMUSYNC_PORT"); p != "" {
		if port, err := strconv.Atoi(p); err == nil {
			cfg.Port = port
		}
	}
	if d := os.Getenv("EMUSYNC_DATA_DIR"); d != "" {
		cfg.DataDir = d
	}
	if m := os.Getenv("EMUSYNC_MAX_BACKUPS"); m != "" {
		if max, err := strconv.Atoi(m); err == nil {
			cfg.MaxBackups = max
		}
	}

	return cfg
}

// Run starts the HTTP server with graceful shutdown.
func Run(cfg ServerConfig) error {
	storage := NewStorage(cfg.DataDir, cfg.MaxBackups)
	handlers := NewHandlers(storage)

	mux := http.NewServeMux()
	handlers.RegisterRoutes(mux)

	var handler http.Handler = mux
	if cfg.AuthToken != "" {
		handler = AuthMiddleware(cfg.AuthToken, mux)
		slog.Info("auth enabled")
	} else {
		slog.Warn("auth disabled (no EMUSYNC_AUTH_TOKEN set)")
	}

	// Add request logging
	handler = loggingMiddleware(handler)

	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      handler,
		ReadTimeout:  5 * time.Minute,
		WriteTimeout: 5 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()
		slog.Info("shutting down server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdownCtx)
	}()

	slog.Info("server starting", "port", cfg.Port, "data_dir", cfg.DataDir)
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		return fmt.Errorf("server error: %w", err)
	}
	return nil
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Debug("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
			"remote", r.RemoteAddr,
		)
	})
}
