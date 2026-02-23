package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/dublin/emusync/internal/config"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate default config file",
	Long:  "Creates a default config.toml at ~/.config/emusync/config.toml with all emulator mappings.",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgPath

		// Check if config already exists
		if _, err := os.Stat(path); err == nil {
			return fmt.Errorf("config already exists at %s (delete it first to regenerate)", path)
		}

		// Create directory
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return fmt.Errorf("creating config directory: %w", err)
		}

		// Write default config
		if err := os.WriteFile(path, []byte(config.DefaultConfigContent()), 0600); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		slog.Info("config created", "path", path)
		fmt.Printf("Config created at %s\n", path)
		fmt.Println("Edit it to set your server address, device ID, and emulator paths.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
