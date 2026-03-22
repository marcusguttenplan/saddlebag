package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "dev"

var rootCmd = &cobra.Command{
	Use:   "sb",
	Short: "Saddlebag — developer context control plane",
	Long: `sb is the CLI companion to Saddlebag.app.

It manages your active developer context (AWS profile, GCP config,
git identity, SSH keys) and keeps your shell in sync.

Quick start:
  eval "$(sb init zsh)"   # add to your .zshrc
  sb health               # check credential status
  sb switch <desk>        # switch context`,
	Version: version,
}

// Execute runs the root command
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
