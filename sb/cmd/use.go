package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/desk"
)

var useClear bool

var useCmd = &cobra.Command{
	Use:   "use <desk>",
	Short: "Set a desk for the current shell session",
	Long: `Print shell export statements for a desk's environment.
This only affects the current shell session.

Usage:
  eval "$(sb use courseclear)"

The shell integration wraps this so you can simply run:
  sb use courseclear

To clear the shell override and re-sync with the resolved desk:
  sb use --clear`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if useClear {
			fmt.Println("unset SADDLEBAG_DESK")
			// Re-sync by printing env from resolved desk
			fmt.Println("eval \"$(command sb env 2>/dev/null)\"")
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("desk name required (or use --clear)")
		}

		deskName := args[0]

		// Load and validate
		desks, err := desk.LoadAll()
		if err != nil {
			return fmt.Errorf("loading desks: %w", err)
		}

		d, ok := desks[deskName]
		if !ok {
			return fmt.Errorf("unknown desk: %s", deskName)
		}

		// Pin this desk to the shell session
		fmt.Printf("export SADDLEBAG_DESK=%q\n", deskName)

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

		// Custom env vars
		for k, v := range d.Env {
			fmt.Printf("export %s=%q\n", k, v)
		}

		return nil
	},
}

func init() {
	useCmd.Flags().BoolVar(&useClear, "clear", false, "Clear shell desk override")
	rootCmd.AddCommand(useCmd)
}
