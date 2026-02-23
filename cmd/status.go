package cmd

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/dublin/emusync/internal/client"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status [emulator]",
	Short: "Show sync status",
	Long:  "Shows which files have changed locally vs. the server.",
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

		anyChanges := false
		for _, emu := range emulators {
			changed, conflicts, err := syncer.Status(ctx, &emu)
			if err != nil {
				fmt.Printf("[%s] error: %s\n", emu.Name, err)
				continue
			}
			if len(changed) == 0 && len(conflicts) == 0 {
				continue
			}
			anyChanges = true
			fmt.Printf("[%s]\n", emu.Name)
			for _, c := range changed {
				fmt.Printf("  M %s\n", c)
			}
			for _, c := range conflicts {
				fmt.Printf("  ! CONFLICT: %s\n", c.Path)
			}
		}

		if !anyChanges {
			fmt.Println("Everything up to date.")
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
