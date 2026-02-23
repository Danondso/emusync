package cmd

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/dublin/emusync/internal/client"
	"github.com/spf13/cobra"
)

var pullCmd = &cobra.Command{
	Use:   "pull [emulator]",
	Short: "Download saves from server",
	Long:  "Downloads latest save files from the server. If no emulator is specified, pulls all.",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncer, err := client.NewSyncer(cfg)
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		emulators := cfg.Emulators
		if len(args) > 0 {
			emulators = filterEmulators(cfg.Emulators, args[0])
			if len(emulators) == 0 {
				return fmt.Errorf("no emulator found matching %q", args[0])
			}
		}

		result, err := syncer.PullAll(ctx, emulators)
		if err != nil {
			return err
		}

		printSyncResult(result)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pullCmd)
}
