package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var switchCmd = &cobra.Command{
	Use:        "switch <desk>",
	Short:      "Switch to a different desk (alias for 'desk global')",
	Args:       cobra.ExactArgs(1),
	Deprecated: "use 'sb desk global <desk>' instead",
	RunE: func(cmd *cobra.Command, args []string) error {
		// Delegate to desk global
		deskGlobalCmd.SetArgs(args)
		if err := deskGlobalCmd.RunE(deskGlobalCmd, args); err != nil {
			return err
		}
		fmt.Println("Note: 'sb switch' is deprecated. Use 'sb desk global' instead.")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}
