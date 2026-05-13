package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/dublin/emusync/internal/config"
	"github.com/dublin/emusync/internal/model"
	"github.com/spf13/cobra"
)

var pullProfileCmd = &cobra.Command{
	Use:   "pull-profile",
	Short: "Fetch emulator profile from server admin API",
	Long: "Calls GET /admin/api/v1/profile with EMUSYNC_ADMIN_TOKEN and replaces client.emulators " +
		"in your config file (other sections unchanged).",
	RunE: func(cmd *cobra.Command, args []string) error {
		adminTok := strings.TrimSpace(os.Getenv("EMUSYNC_ADMIN_TOKEN"))
		if adminTok == "" {
			return fmt.Errorf("EMUSYNC_ADMIN_TOKEN must be set in the environment")
		}

		url := strings.TrimRight(cfg.Server.BaseURL(), "/") + "/admin/api/v1/profile"
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			return err
		}
		req.Header.Set("Authorization", "Bearer "+adminTok)

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return fmt.Errorf("admin profile request: %w", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return err
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("admin profile: HTTP %s: %s", resp.Status, strings.TrimSpace(string(body)))
		}

		var doc struct {
			Version   int                    `json:"version"`
			Emulators []model.EmulatorConfig `json:"emulators"`
		}
		if err := json.Unmarshal(body, &doc); err != nil {
			return fmt.Errorf("decode profile JSON: %w", err)
		}
		if doc.Version != 1 {
			return fmt.Errorf("unsupported profile version %d", doc.Version)
		}

		cfg.Emulators = append([]model.EmulatorConfig(nil), doc.Emulators...)
		if err := config.Save(cfgPath, cfg); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "Updated %d emulator mapping(s) in %s\n", len(cfg.Emulators), cfgPath)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pullProfileCmd)
}
