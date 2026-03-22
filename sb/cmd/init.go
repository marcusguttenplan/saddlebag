package cmd

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
)

//go:embed shell/sb.zsh
var shellScript string

var initCmd = &cobra.Command{
	Use:   "init [shell]",
	Short: "Print shell integration script",
	Long:  `Print the shell integration script for your shell. Currently only zsh is supported.`,
	Args:  cobra.ExactArgs(1),
	ValidArgs: []string{"zsh"},
	RunE: func(cmd *cobra.Command, args []string) error {
		switch args[0] {
		case "zsh":
			fmt.Print(shellScript)
			return nil
		default:
			return fmt.Errorf("unsupported shell: %s (supported: zsh)", args[0])
		}
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
