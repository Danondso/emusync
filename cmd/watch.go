package cmd

import (
	"context"
	"fmt"
	"log/slog"
	"os/signal"
	"syscall"
	"time"

	"github.com/dublin/emusync/internal/client"
	"github.com/dublin/emusync/internal/watcher"
	"github.com/spf13/cobra"
)

var watchCmd = &cobra.Command{
	Use:   "watch",
	Short: "Watch for emulator processes and auto-sync",
	Long:  "Monitors running processes and automatically syncs saves when an emulator exits or launches.",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncer, err := client.NewSyncer(cfg)
		if err != nil {
			return err
		}
		pollInterval := time.Duration(cfg.Sync.PollIntervalMs) * time.Millisecond
		postExitDelay := time.Duration(cfg.Sync.PostExitDelayMs) * time.Millisecond

		w := watcher.NewWatcher(cfg.Emulators, pollInterval)

		ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
		defer stop()

		// Start watcher in background
		go func() {
			if err := w.Run(ctx); err != nil && ctx.Err() == nil {
				slog.Error("watcher error", "error", err)
			}
		}()

		slog.Info("emusync watch started",
			"device_id", cfg.Client.DeviceID,
			"emulators", len(cfg.Emulators),
			"poll_ms", cfg.Sync.PollIntervalMs,
		)
		fmt.Printf("Watching for %d emulator(s)...\n", len(cfg.Emulators))

		// Process events
		for event := range w.Events() {
			switch event.EventType {
			case watcher.Launched:
				if cfg.Sync.AutoSyncOnLaunch {
					slog.Info("pre-launch sync", "emulator", event.Emulator.Name, "pid", event.PID)
					result, err := syncer.SyncBeforeLaunch(ctx, event.Emulator)
					if err != nil {
						slog.Error("pre-launch sync failed", "emulator", event.Emulator.Name, "error", err)
					} else if len(result.Downloaded) > 0 {
						slog.Info("pre-launch sync complete",
							"emulator", event.Emulator.Name,
							"downloaded", len(result.Downloaded),
						)
					}
				}

			case watcher.Exited:
				if cfg.Sync.AutoSyncOnClose {
					go func(evt watcher.ProcessEvent) {
						// Wait for emulator to finish flushing saves
						slog.Debug("waiting for save flush", "delay", postExitDelay)
						time.Sleep(postExitDelay)

						slog.Info("post-exit sync", "emulator", evt.Emulator.Name, "pid", evt.PID)
						result, err := syncer.SyncAfterExit(ctx, evt.Emulator)
						if err != nil {
							slog.Error("post-exit sync failed", "emulator", evt.Emulator.Name, "error", err)
						} else {
							if len(result.Uploaded) > 0 {
								slog.Info("post-exit sync complete",
									"emulator", evt.Emulator.Name,
									"uploaded", len(result.Uploaded),
								)
							}
							if len(result.Conflicts) > 0 {
								slog.Warn("conflicts detected",
									"emulator", evt.Emulator.Name,
									"count", len(result.Conflicts),
								)
							}
						}
					}(event)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(watchCmd)
}
