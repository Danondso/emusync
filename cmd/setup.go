package cmd

import (
	"os"

	"github.com/dublin/emusync/internal/setup"
	"github.com/spf13/cobra"
)

var setupForce bool

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Interactive client configuration",
	Long:  "Discovers servers on the LAN (mDNS), then prompts for auth token and save directories.",
	RunE: func(cmd *cobra.Command, args []string) error {
		path := cfgPath
		return setup.RunWizard(setup.WizardOptions{
			CfgPath: path,
			Force:   setupForce,
			In:      os.Stdin,
			Out:     cmd.OutOrStdout(),
			ErrOut:  cmd.OutOrStderr(),
		})
	},
}

func init() {
	setupCmd.Flags().BoolVar(&setupForce, "force", false, "overwrite existing config file")
	rootCmd.AddCommand(setupCmd)
}
