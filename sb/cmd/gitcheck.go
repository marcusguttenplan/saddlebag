package cmd

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/desk"
	"github.com/marcusguttenplan/sb/internal/state"
)

var gitCheckCmd = &cobra.Command{
	Use:   "git-check",
	Short: "Verify git user.email matches active desk",
	Long: `Check that the current git user.email matches the expected email
from the active desk. Used as a pre-commit hook to prevent commits
with the wrong identity.

Exit code 0 = match, exit code 1 = mismatch or no desk active.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := state.Read()
		if err != nil {
			return fmt.Errorf("reading state: %w", err)
		}

		if s.ActiveDesk == "" {
			fmt.Println("⚠️  No active desk — skipping git identity check")
			return nil
		}

		// Load the desk to get expected email
		desks, err := desk.LoadAll()
		if err != nil {
			return fmt.Errorf("loading desks: %w", err)
		}

		d, ok := desks[s.ActiveDesk]
		if !ok {
			return fmt.Errorf("active desk %q not found in ~/.saddlebag/desks/", s.ActiveDesk)
		}

		if d.Git == nil || d.Git.Email == "" {
			fmt.Println("⚠️  Desk has no git email configured — skipping check")
			return nil
		}

		// Get the current git user.email
		out, err := exec.Command("git", "config", "user.email").CombinedOutput()
		if err != nil {
			return fmt.Errorf("getting git user.email: %w", err)
		}

		actual := strings.TrimSpace(string(out))
		expected := d.Git.Email

		if !strings.EqualFold(actual, expected) {
			return fmt.Errorf(
				"✋ Git email mismatch!\n"+
					"   Expected: %s (desk: %s)\n"+
					"   Actual:   %s\n\n"+
					"   Fix with: git config user.email %q\n"+
					"   Or switch desk: sb switch %s",
				expected, s.ActiveDesk, actual, expected, s.ActiveDesk,
			)
		}

		fmt.Printf("✅ Git email matches desk %q (%s)\n", s.ActiveDesk, expected)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(gitCheckCmd)
}
