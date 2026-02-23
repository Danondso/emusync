package cmd

import (
	"fmt"

	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/logging"
	"github.com/spf13/cobra"
)

var (
	cfgPath    string
	verbose    bool
	cfg        *config.Config
	logCleanup func()
)

var rootCmd = &cobra.Command{
	Use:   "emusync",
	Short: "Emulator save file sync",
	Long:  "Self-hosted emulator save file sync across devices.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		// Server command reads env vars, not config file
		if cmd.Name() == "server" {
			return nil
		}

		logPath, err := config.DefaultLogPath()
		if err != nil {
			return fmt.Errorf("determining log path: %w", err)
		}

		// Init command doesn't need an existing config
		if cmd.Name() == "init" {
			cleanup, err := logging.Setup(logPath, verbose)
			if err != nil {
				return err
			}
			logCleanup = cleanup
			return nil
		}

		cleanup, err := logging.Setup(logPath, verbose)
		if err != nil {
			return fmt.Errorf("setting up logging: %w", err)
		}
		logCleanup = cleanup

		cfg, err = config.Load(cfgPath)
		if err != nil {
			return fmt.Errorf("loading config: %w", err)
		}
		return nil
	},
}

func init() {
	defaultCfgPath, _ := config.DefaultConfigPath()
	rootCmd.PersistentFlags().StringVar(&cfgPath, "config", defaultCfgPath, "config file path")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "enable debug logging")
}

// Execute runs the root command.
func Execute() error {
	defer func() {
		if logCleanup != nil {
			logCleanup()
		}
	}()
	return rootCmd.Execute()
}
