package cmd

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/dublin/emusync/internal/client"
	"github.com/spf13/cobra"
)

var resolveCmd = &cobra.Command{
	Use:   "resolve",
	Short: "Resolve sync conflicts",
	Long:  "Interactively resolve unresolved save file conflicts.",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncer, err := client.NewSyncer(cfg)
		if err != nil {
			return err
		}
		ctx, cancel := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
		defer cancel()

		conflicts, err := syncer.GetClient().GetConflicts(ctx)
		if err != nil {
			return fmt.Errorf("getting conflicts: %w", err)
		}

		if len(conflicts) == 0 {
			fmt.Println("No unresolved conflicts.")
			return nil
		}

		fmt.Printf("Found %d unresolved conflict(s):\n\n", len(conflicts))

		reader := bufio.NewReader(os.Stdin)

		for i, c := range conflicts {
			fmt.Printf("--- Conflict %d/%d ---\n", i+1, len(conflicts))
			fmt.Printf("  Emulator: %s\n", c.Emulator)
			fmt.Printf("  File:     %s\n", c.Path)
			fmt.Printf("\n")
			fmt.Printf("  LOCAL  (incoming):  device=%s  time=%s  size=%d bytes  sha=%s\n",
				c.Local.DeviceID,
				c.Local.Timestamp.Local().Format("2006-01-02 15:04:05"),
				c.Local.Size,
				truncHash(c.Local.SHA256),
			)
			fmt.Printf("  REMOTE (on server): device=%s  time=%s  size=%d bytes  sha=%s\n",
				c.Remote.DeviceID,
				c.Remote.Timestamp.Local().Format("2006-01-02 15:04:05"),
				c.Remote.Size,
				truncHash(c.Remote.SHA256),
			)
			fmt.Printf("\n")
			fmt.Printf("  Choose: (l)ocal / (r)emote / (k)eep both / (s)kip? ")

			input, _ := reader.ReadString('\n')
			input = strings.TrimSpace(strings.ToLower(input))

			var choice string
			switch input {
			case "l", "local":
				choice = "local"
			case "r", "remote":
				choice = "remote"
			case "k", "keep", "keep-both":
				choice = "keep-both"
			case "s", "skip":
				fmt.Println("  Skipped.")
				continue
			default:
				fmt.Println("  Invalid choice, skipping.")
				continue
			}

			if err := syncer.GetClient().ResolveConflict(ctx, c.ID, choice); err != nil {
				fmt.Printf("  Error resolving: %s\n", err)
			} else {
				fmt.Printf("  Resolved: %s\n", choice)
			}
			fmt.Println()
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(resolveCmd)
}

func truncHash(hash string) string {
	if len(hash) > 12 {
		return hash[:12] + "..."
	}
	return hash
}
