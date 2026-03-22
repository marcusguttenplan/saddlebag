package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/state"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Print the active AWS profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := state.Read()
		if err != nil {
			return fmt.Errorf("reading state: %w", err)
		}

		if s.AWSProfile == "" {
			fmt.Println("(none)")
		} else {
			fmt.Println(s.AWSProfile)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(profileCmd)
}
