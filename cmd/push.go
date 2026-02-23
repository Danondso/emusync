package cmd

import (
	"fmt"
	"os/signal"
	"syscall"

	"github.com/dublin/emusync/internal/client"
	"github.com/dublin/emusync/internal/model"
	"github.com/spf13/cobra"
)

var pushCmd = &cobra.Command{
	Use:   "push [emulator]",
	Short: "Upload saves to server",
	Long:  "Uploads changed save files to the server. If no emulator is specified, pushes all.",
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

		result, err := syncer.PushAll(ctx, emulators)
		if err != nil {
			return err
		}

		printSyncResult(result)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pushCmd)
}

func filterEmulators(emulators []model.EmulatorConfig, name string) []model.EmulatorConfig {
	for _, e := range emulators {
		if e.Name == name {
			return []model.EmulatorConfig{e}
		}
	}
	return nil
}

func printSyncResult(result *model.SyncResult) {
	if len(result.Uploaded) > 0 {
		fmt.Printf("Uploaded %d file(s):\n", len(result.Uploaded))
		for _, f := range result.Uploaded {
			fmt.Printf("  + %s\n", f)
		}
	}
	if len(result.Downloaded) > 0 {
		fmt.Printf("Downloaded %d file(s):\n", len(result.Downloaded))
		for _, f := range result.Downloaded {
			fmt.Printf("  - %s\n", f)
		}
	}
	if len(result.Conflicts) > 0 {
		fmt.Printf("Conflicts detected: %d\n", len(result.Conflicts))
		for _, c := range result.Conflicts {
			fmt.Printf("  ! %s/%s (run 'emusync resolve' to fix)\n", c.Emulator, c.Path)
		}
	}
	if len(result.Errors) > 0 {
		fmt.Printf("Errors: %d\n", len(result.Errors))
		for _, e := range result.Errors {
			fmt.Printf("  E %s\n", e)
		}
	}
	if len(result.Uploaded) == 0 && len(result.Downloaded) == 0 && len(result.Conflicts) == 0 && len(result.Errors) == 0 {
		fmt.Println("Everything up to date.")
	}
}
