package cmd

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/desk"
	"github.com/marcusguttenplan/sb/internal/ipc"
	"github.com/marcusguttenplan/sb/internal/state"
)

var deskCmd = &cobra.Command{
	Use:   "desk",
	Short: "Manage desk context (global, local, or show current)",
	Long: `Manage desk context using a three-tier resolution model:

  sb desk              Show the currently resolved desk and its tier
  sb desk global <d>   Set the machine-wide default desk
  sb desk local <d>    Set a per-directory desk (writes .desk file)

Resolution priority: shell ($SADDLEBAG_DESK) → local (.desk) → global (state.json)`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// No subcommand: show current resolved desk
		deskID, tier, source := resolveCurrentDesk()
		if deskID == "" {
			fmt.Println("No active desk")
			return nil
		}
		fmt.Printf("%s (set by %s)\n", deskID, source)
		_ = tier
		return nil
	},
}

var deskGlobalCmd = &cobra.Command{
	Use:   "global <desk>",
	Short: "Set the machine-wide default desk",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deskName := args[0]

		// Validate desk exists
		desks, err := desk.LoadAll()
		if err != nil {
			return fmt.Errorf("loading desks: %w", err)
		}
		if _, ok := desks[deskName]; !ok {
			return fmt.Errorf("unknown desk: %s", deskName)
		}

		// Use IPC to switch via the app (sets AWS, GCP, SSH, writes state)
		resp, err := ipc.Send(context.Background(), ipc.Command{
			Action: "switch",
			Desk:   deskName,
		})
		if err != nil {
			// Check if it's a connection error (app not running) vs app error
			var netErr *net.OpError
			if errors.As(err, &netErr) {
				// App not running — fall back to writing state directly
				s, _ := state.Read()
				s.ActiveDesk = deskName
				if d, ok := desks[deskName]; ok {
					if d.AWS != nil {
						s.AWSProfile = d.AWS.Profile
					}
					if d.GCP != nil {
						s.GCPConfig = d.GCP.Config
					}
					if d.Git != nil {
						s.GitEmail = d.Git.Email
					}
				}
				if writeErr := state.Write(s); writeErr != nil {
					return fmt.Errorf("writing state: %w", writeErr)
				}
				fmt.Printf("Set global desk to %s (app not running, wrote state directly)\n", deskName)
				return nil
			}
			// App returned an error
			return fmt.Errorf("switching desk: %w", err)
		}

		fmt.Printf("Set global desk to %s: %s\n", deskName, resp.Message)
		return nil
	},
}

var deskLocalCmd = &cobra.Command{
	Use:   "local <desk>",
	Short: "Set a per-directory desk (writes .desk file in cwd)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deskName := args[0]

		// Validate desk exists
		desks, err := desk.LoadAll()
		if err != nil {
			return fmt.Errorf("loading desks: %w", err)
		}
		if _, ok := desks[deskName]; !ok {
			return fmt.Errorf("unknown desk: %s", deskName)
		}

		cwd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("getting cwd: %w", err)
		}

		if err := desk.WriteDeskFile(cwd, deskName); err != nil {
			return fmt.Errorf("writing .desk: %w", err)
		}

		fmt.Printf("Set local desk to %s (wrote %s/.desk)\n", deskName, cwd)
		return nil
	},
}

func resolveCurrentDesk() (deskID string, tier desk.ResolveTier, source string) {
	// Tier 1: Shell override
	if env := os.Getenv("SADDLEBAG_DESK"); env != "" {
		return env, desk.TierShell, "$SADDLEBAG_DESK"
	}

	// Tier 2: Local .desk file
	if cwd, err := os.Getwd(); err == nil {
		if id, foundAt := desk.FindDeskFile(cwd); id != "" {
			return id, desk.TierLocal, foundAt
		}
	}

	// Tier 3: Global state
	if s, err := state.Read(); err == nil && s.ActiveDesk != "" {
		return s.ActiveDesk, desk.TierGlobal, "~/.saddlebag/state.json"
	}

	return "", desk.TierNone, ""
}

func init() {
	deskCmd.AddCommand(deskGlobalCmd)
	deskCmd.AddCommand(deskLocalCmd)
	rootCmd.AddCommand(deskCmd)
}
