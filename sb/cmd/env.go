package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/desk"
	"github.com/marcusguttenplan/sb/internal/state"
)

var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Print managed environment variables as shell exports",
	Long: `Print shell export statements for the resolved desk's environment.

Resolution priority: shell ($SADDLEBAG_DESK) → local (.desk) → global (state.json)

Designed to be used with eval:
  eval "$(sb env)"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Three-tier resolution
		deskID, tier := resolveDeskForEnv()

		if deskID == "" {
			return nil // No desk active
		}

		// Load the resolved desk
		desks, err := desk.LoadAll()
		if err != nil {
			return nil // non-fatal
		}

		d, ok := desks[deskID]
		if !ok {
			// Desk in state but TOML deleted — fall back to state for basic fields
			if tier == desk.TierGlobal {
				return printFromState()
			}
			return nil
		}

		// Export desk identity
		if tier != desk.TierShell {
			// Don't re-export if already set by sb use
			fmt.Printf("export SADDLEBAG_DESK=%q\n", deskID)
		}

		// AWS
		if d.AWS != nil && d.AWS.Profile != "" {
			fmt.Printf("export AWS_PROFILE=%q\n", d.AWS.Profile)
		}

		// GCP
		if d.GCP != nil && d.GCP.Config != "" {
			fmt.Printf("export CLOUDSDK_ACTIVE_CONFIG_NAME=%q\n", d.GCP.Config)
		}

		// Git identity
		if d.Git != nil {
			if d.Git.Email != "" {
				fmt.Printf("export GIT_AUTHOR_EMAIL=%q\n", d.Git.Email)
				fmt.Printf("export GIT_COMMITTER_EMAIL=%q\n", d.Git.Email)
			}
			if d.Git.Name != "" {
				fmt.Printf("export GIT_AUTHOR_NAME=%q\n", d.Git.Name)
				fmt.Printf("export GIT_COMMITTER_NAME=%q\n", d.Git.Name)
			}
		}

		// Custom env vars from [env] section
		for k, v := range d.Env {
			fmt.Printf("export %s=%q\n", k, v)
		}

		return nil
	},
}

func resolveDeskForEnv() (string, desk.ResolveTier) {
	// Tier 1: Shell override
	if env := os.Getenv("SADDLEBAG_DESK"); env != "" {
		return env, desk.TierShell
	}

	// Tier 2: Local .desk file
	if cwd, err := os.Getwd(); err == nil {
		if id, _ := desk.FindDeskFile(cwd); id != "" {
			return id, desk.TierLocal
		}
	}

	// Tier 3: Global state
	if s, err := state.Read(); err == nil && s.ActiveDesk != "" {
		return s.ActiveDesk, desk.TierGlobal
	}

	return "", desk.TierNone
}

// printFromState handles legacy case where state has fields but TOML is missing
func printFromState() error {
	s, err := state.Read()
	if err != nil {
		return nil
	}
	if s.AWSProfile != "" {
		fmt.Printf("export AWS_PROFILE=%q\n", s.AWSProfile)
	}
	if s.GCPConfig != "" {
		fmt.Printf("export CLOUDSDK_ACTIVE_CONFIG_NAME=%q\n", s.GCPConfig)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(envCmd)
}
