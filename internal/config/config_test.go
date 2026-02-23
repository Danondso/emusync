package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTOML(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_Valid(t *testing.T) {
	path := writeTOML(t, `
[client]
device_id = "my-deck"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Client.DeviceID != "my-deck" {
		t.Errorf("device_id = %q, want %q", cfg.Client.DeviceID, "my-deck")
	}
}

func TestLoad_AppliesDefaults(t *testing.T) {
	path := writeTOML(t, `
[client]
device_id = "test-device"
`)
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Server.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Server.Port)
	}
	if cfg.Client.SavesPath != "~/Emulation/saves" {
		t.Errorf("SavesPath = %q, want %q", cfg.Client.SavesPath, "~/Emulation/saves")
	}
	if cfg.Client.MaxLocalBackups != 10 {
		t.Errorf("MaxLocalBackups = %d, want 10", cfg.Client.MaxLocalBackups)
	}
	if cfg.Sync.ConflictStrategy != "prompt" {
		t.Errorf("ConflictStrategy = %q, want %q", cfg.Sync.ConflictStrategy, "prompt")
	}
	if cfg.Sync.PollIntervalMs != 2000 {
		t.Errorf("PollIntervalMs = %d, want 2000", cfg.Sync.PollIntervalMs)
	}
	if cfg.Sync.PostExitDelayMs != 2000 {
		t.Errorf("PostExitDelayMs = %d, want 2000", cfg.Sync.PostExitDelayMs)
	}
}

func TestLoad_MissingDeviceID(t *testing.T) {
	path := writeTOML(t, `
[server]
port = 9090
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for missing device_id, got nil")
	}
	if !strings.Contains(err.Error(), "device_id") {
		t.Errorf("error %q should contain %q", err.Error(), "device_id")
	}
}

func TestLoad_InvalidConflictStrategy(t *testing.T) {
	path := writeTOML(t, `
[client]
device_id = "test"

[sync]
conflict_strategy = "overwrite"
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for invalid conflict_strategy, got nil")
	}
}

func TestLoad_ValidStrategies(t *testing.T) {
	strategies := []string{"prompt", "newest", "keep-both"}

	for _, strategy := range strategies {
		t.Run(strategy, func(t *testing.T) {
			path := writeTOML(t, `
[client]
device_id = "test"

[sync]
conflict_strategy = "`+strategy+`"
`)
			_, err := Load(path)
			if err != nil {
				t.Fatalf("strategy %q should be valid, got error: %v", strategy, err)
			}
		})
	}
}

func TestLoad_MalformedTOML(t *testing.T) {
	path := writeTOML(t, `
this is not valid toml [[[
`)
	_, err := Load(path)
	if err == nil {
		t.Fatal("expected error for malformed TOML, got nil")
	}
}

func TestLoad_NonexistentFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.toml")
	if err == nil {
		t.Fatal("expected error for nonexistent file, got nil")
	}
}

func TestBaseURL(t *testing.T) {
	s := &ServerConfig{
		Host: "192.168.1.1",
		Port: 8080,
	}
	got := s.BaseURL()
	want := "http://192.168.1.1:8080"
	if got != want {
		t.Errorf("BaseURL() = %q, want %q", got, want)
	}
}

func TestResolveSavePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}

	cfg := &Config{
		Client: ClientConfig{
			SavesPath: "~/Emulation/saves",
		},
	}

	t.Run("absolute_returned_as_is", func(t *testing.T) {
		got := cfg.ResolveSavePath("/opt/saves/game")
		if got != "/opt/saves/game" {
			t.Errorf("got %q, want %q", got, "/opt/saves/game")
		}
	})

	t.Run("relative_joined_with_saves_path", func(t *testing.T) {
		got := cfg.ResolveSavePath("retroarch/saves")
		want := filepath.Join(home, "Emulation", "saves", "retroarch", "saves")
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
	})
}
