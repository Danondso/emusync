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
	Short: "Generate minimal starter config",
	Long:  "Creates a small config.toml without emulator mappings. Run emusync setup to attach to your server and tune paths.",
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

		hostName, err := os.Hostname()
		if err != nil {
			hostName = "my-device"
		}
		cfg := config.MinimalSkeleton(hostName)
		if err := config.Save(path, cfg); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}

		slog.Info("config created", "path", path)
		fmt.Printf("Starter config created at %s\n", path)
		fmt.Println("Run emusync setup to connect to your server and configure save paths.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
