package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/desk"
	"github.com/marcusguttenplan/sb/internal/state"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Print managed environment variables as shell exports",
	Long: `Print shell export statements for the active desk's environment.

Designed to be used with eval:
  eval "$(sb env)"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := state.Read()
		if err != nil {
			return fmt.Errorf("reading state: %w", err)
		}

		// Always export the AWS profile if set
		if s.AWSProfile != "" {
			fmt.Printf("export AWS_PROFILE=%q\n", s.AWSProfile)
		}

		// Always export GCP config if set
		if s.GCPConfig != "" {
			fmt.Printf("export CLOUDSDK_ACTIVE_CONFIG_NAME=%q\n", s.GCPConfig)
		}

		// If there's an active desk, export its env vars too
		if s.ActiveDesk != "" {
			desks, err := desk.LoadAll()
			if err != nil {
				return nil // non-fatal: just skip desk env vars
			}

			if d, ok := desks[s.ActiveDesk]; ok {
				for k, v := range d.Env {
					fmt.Printf("export %s=%q\n", k, v)
				}
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(envCmd)
}
