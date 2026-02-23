package cmd

import (
	"log/slog"
	"os"

	"github.com/dublin/emusync/internal/server"
	"github.com/spf13/cobra"
)

var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Start the emusync server",
	Long:  "Starts the HTTP API server for receiving and serving save files. Typically run inside Docker.",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Server uses env vars for config (Docker-friendly)
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})))

		cfg := server.ConfigFromEnv()
		return server.Run(cfg)
	},
}

func init() {
	rootCmd.AddCommand(serverCmd)
}
