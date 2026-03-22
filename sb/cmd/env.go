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

Resolution priority: shell ($SADDLEBAG_DESK) → local (.desk) → workdir (desk config) → global (state.json)

Designed to be used with eval:
  eval "$(sb env)"`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// Four-tier resolution
		deskID, tier := resolveDeskForEnv()

		if deskID == "" {
			// No desk — clear theme vars
			fmt.Println("unset SADDLEBAG_DESK_ID SADDLEBAG_DESK_NAME SADDLEBAG_DESK_TIER SADDLEBAG_AWS_PROFILE SADDLEBAG_GCP_CONFIG SADDLEBAG_GIT_EMAIL 2>/dev/null")
			return nil
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

		// --- Functional env vars ---

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

		// --- Theme vars (SADDLEBAG_* for prompt/theme consumption) ---
		// Note: SADDLEBAG_DESK is only set by `sb use` (shell pin).
		// SADDLEBAG_DESK_ID is the informational current desk.

		fmt.Printf("export SADDLEBAG_DESK_ID=%q\n", deskID)
		fmt.Printf("export SADDLEBAG_DESK_NAME=%q\n", d.DeskMeta.Name)
		fmt.Printf("export SADDLEBAG_DESK_TIER=%q\n", string(tier))

		if d.AWS != nil && d.AWS.Profile != "" {
			fmt.Printf("export SADDLEBAG_AWS_PROFILE=%q\n", d.AWS.Profile)
		} else {
			fmt.Println("unset SADDLEBAG_AWS_PROFILE 2>/dev/null")
		}

		if d.GCP != nil && d.GCP.Config != "" {
			fmt.Printf("export SADDLEBAG_GCP_CONFIG=%q\n", d.GCP.Config)
		} else {
			fmt.Println("unset SADDLEBAG_GCP_CONFIG 2>/dev/null")
		}

		if d.Git != nil && d.Git.Email != "" {
			fmt.Printf("export SADDLEBAG_GIT_EMAIL=%q\n", d.Git.Email)
		} else {
			fmt.Println("unset SADDLEBAG_GIT_EMAIL 2>/dev/null")
		}

		return nil
	},
}

func resolveDeskForEnv() (string, desk.ResolveTier) {
	// Tier 1: Shell pin ($SADDLEBAG_DESK, only set by `sb use`)
	if env := os.Getenv("SADDLEBAG_DESK"); env != "" {
		return env, desk.TierShell
	}

	// Tier 2: Local .desk file
	if cwd, err := os.Getwd(); err == nil {
		if id, _ := desk.FindDeskFile(cwd); id != "" {
			return id, desk.TierLocal
		}

		// Tier 3: Working directory match (desk config)
		if id := desk.FindDeskByWorkingDir(cwd); id != "" {
			return id, desk.TierWorkdir
		}
	}

	// Tier 4: Global state
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
