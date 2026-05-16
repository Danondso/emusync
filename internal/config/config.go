package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"github.com/dublin/emusync/internal/authtoken"
	"github.com/dublin/emusync/internal/model"
)

// Config is the top-level configuration.
type Config struct {
	Server    ServerConfig           `toml:"server"`
	Client    ClientConfig           `toml:"client"`
	Sync      SyncConfig             `toml:"sync"`
	Emulators []model.EmulatorConfig `toml:"emulators"`
}

// ServerConfig holds server connection settings.
type ServerConfig struct {
	Host      string `toml:"host"`
	Port      int    `toml:"port"`
	AuthToken string `toml:"auth_token"`
}

// ClientConfig holds client-side settings.
type ClientConfig struct {
	DeviceID        string `toml:"device_id"`
	SavesPath       string `toml:"saves_path"`
	BackupPath      string `toml:"backup_path"`
	MaxLocalBackups int    `toml:"max_local_backups"`
}

// SyncConfig holds sync behavior settings.
type SyncConfig struct {
	AutoSyncOnClose  bool   `toml:"auto_sync_on_close"`
	AutoSyncOnLaunch bool   `toml:"auto_sync_on_launch"`
	ConflictStrategy string `toml:"conflict_strategy"`
	PollIntervalMs   int    `toml:"poll_interval_ms"`
	PostExitDelayMs  int    `toml:"post_exit_delay_ms"`
}

// BaseURL returns the full server base URL.
func (s *ServerConfig) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", s.Host, s.Port)
}

// ExpandedSavesPath returns the saves path with ~ expanded.
func (c *ClientConfig) ExpandedSavesPath() string {
	return expandHome(c.SavesPath)
}

// ExpandedBackupPath returns the backup path with ~ expanded.
func (c *ClientConfig) ExpandedBackupPath() string {
	return expandHome(c.BackupPath)
}

// DefaultConfigPath returns the default config file location.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".config", "emusync", "config.toml"), nil
}

// DefaultStatePath returns the default state file location.
func DefaultStatePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "emusync", "state.json"), nil
}

// DefaultLogPath returns the default log file location.
func DefaultLogPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("getting home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share", "emusync", "sync.log"), nil
}

// Load reads and parses a TOML config file.
func Load(path string) (*Config, error) {
	cfg := &Config{}
	if _, err := toml.DecodeFile(path, cfg); err != nil {
		return nil, fmt.Errorf("loading config %s: %w", path, err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("validating config: %w", err)
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	c.Server.AuthToken = authtoken.Normalize(c.Server.AuthToken)
	if strings.TrimSpace(c.Server.Host) == "" {
		c.Server.Host = "127.0.0.1"
	}
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Client.SavesPath == "" {
		c.Client.SavesPath = "~/Emulation/saves"
	}
	if c.Client.BackupPath == "" {
		base := expandHome(c.Client.SavesPath)
		c.Client.BackupPath = pathToTilde(filepath.Join(base, ".sync-backups"))
	}
	if c.Client.MaxLocalBackups == 0 {
		c.Client.MaxLocalBackups = 10
	}
	if c.Sync.ConflictStrategy == "" {
		c.Sync.ConflictStrategy = "prompt"
	}
	if c.Sync.PollIntervalMs == 0 {
		c.Sync.PollIntervalMs = 2000
	}
	if c.Sync.PostExitDelayMs == 0 {
		c.Sync.PostExitDelayMs = 2000
	}
}

func (c *Config) validate() error {
	if c.Client.DeviceID == "" {
		return fmt.Errorf("client.device_id is required")
	}
	switch c.Sync.ConflictStrategy {
	case "prompt", "newest", "keep-both":
		// valid
	default:
		return fmt.Errorf("sync.conflict_strategy must be one of: prompt, newest, keep-both")
	}
	return nil
}

// ResolveSavePath resolves an emulator save path relative to the base saves path.
// If the save path is absolute, it is returned as-is.
func (c *Config) ResolveSavePath(savePath string) string {
	expanded := expandHome(savePath)
	if filepath.IsAbs(expanded) {
		return expanded
	}
	return filepath.Join(c.Client.ExpandedSavesPath(), expanded)
}

func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// Normalize applies default values to unset fields.
func (c *Config) Normalize() {
	c.applyDefaults()
}

func pathToTilde(abs string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return abs
	}
	abs = filepath.Clean(abs)
	home = filepath.Clean(home)
	if abs == home {
		return "~"
	}
	prefix := home + string(filepath.Separator)
	if strings.HasPrefix(abs, prefix) {
		return filepath.Join("~", strings.TrimPrefix(abs, prefix))
	}
	return abs
}

// Save writes cfg as TOML to path (0600), replacing atomically.
func Save(path string, cfg *Config) error {
	toSave := *cfg
	if len(cfg.Emulators) > 0 {
		toSave.Emulators = append([]model.EmulatorConfig(nil), cfg.Emulators...)
	}
	toSave.Normalize()
	if err := toSave.validate(); err != nil {
		return fmt.Errorf("validating config: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	encodeErr := func() error {
		enc := toml.NewEncoder(f)
		if err := enc.Encode(&toSave); err != nil {
			return fmt.Errorf("encoding config: %w", err)
		}
		return nil
	}()
	if encodeErr != nil {
		f.Close()
		_ = os.Remove(tmp)
		return encodeErr
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("renaming config: %w", err)
	}
	return nil
}

// MinimalSkeleton returns a tiny starter config (run emusync setup to finish).
func MinimalSkeleton(hostname string) *Config {
	devID := SanitizeDeviceID(hostname)
	cfg := &Config{
		Server: ServerConfig{
			Host:      "127.0.0.1",
			Port:      8080,
			AuthToken: "configure-with-emusync-setup",
		},
		Client: ClientConfig{
			DeviceID:   devID,
			SavesPath:  "~/Emulation/saves",
			BackupPath: "",
		},
		Sync: SyncConfig{
			AutoSyncOnClose:  true,
			AutoSyncOnLaunch: true,
			ConflictStrategy: "prompt",
			PollIntervalMs:   2000,
			PostExitDelayMs:  2000,
		},
		Emulators: nil,
	}
	cfg.Normalize()
	return cfg
}

// SanitizeDeviceID yields a safe client.device_id from the OS hostname.
func SanitizeDeviceID(raw string) string {
	raw = strings.TrimSpace(strings.ToLower(raw))
	var b strings.Builder
	for _, r := range raw {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			b.WriteRune(r)
		}
	}
	s := b.String()
	if s == "" {
		return "my-device"
	}
	return s
}
