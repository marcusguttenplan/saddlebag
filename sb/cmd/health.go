package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/health"
)

var healthCmd = &cobra.Command{
	Use:   "health",
	Short: "Check credential status across all services",
	Run: func(cmd *cobra.Command, args []string) {
		checks := []health.Status{
			health.CheckAWSSSO(),
			health.CheckGCloudAuth(),
			health.CheckSSHAgent(),
		}

		for _, c := range checks {
			fmt.Printf("%s  %-12s %s\n", c.Emoji(), c.Service, c.Detail)
		}
	},
}

func init() {
	rootCmd.AddCommand(healthCmd)
}
