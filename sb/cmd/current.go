package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Print the active desk name",
	RunE: func(cmd *cobra.Command, args []string) error {
		deskID, _, _ := resolveCurrentDesk()
		if deskID == "" {
			fmt.Println("(none)")
		} else {
			fmt.Println(deskID)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(currentCmd)
}
