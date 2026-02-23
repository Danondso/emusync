package cmd

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/dublin/emusync/internal/client"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:   "history <emulator> <path>",
	Short: "Show version history for a save file",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		syncer, err := client.NewSyncer(cfg)
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		emulator := args[0]
		path := args[1]

		versions, err := syncer.GetClient().GetHistory(ctx, emulator, path)
		if err != nil {
			return fmt.Errorf("getting history: %w", err)
		}

		if len(versions) == 0 {
			fmt.Println("No version history found.")
			return nil
		}

		fmt.Printf("Version history for %s/%s:\n\n", emulator, path)
		for i, v := range versions {
			label := "  "
			if i == 0 {
				label = "> "
			}
			fmt.Printf("%s%s  %s  %6d bytes  device: %s\n",
				label,
				v.Timestamp.Local().Format("2006-01-02 15:04:05"),
				truncHash(v.SHA256),
				v.Size,
				v.DeviceID,
			)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(historyCmd)
}
