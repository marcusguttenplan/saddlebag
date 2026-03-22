package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/state"
)

var currentCmd = &cobra.Command{
	Use:   "current",
	Short: "Print the active desk name",
	RunE: func(cmd *cobra.Command, args []string) error {
		s, err := state.Read()
		if err != nil {
			return fmt.Errorf("reading state: %w", err)
		}

		if s.ActiveDesk == "" {
			fmt.Println("(none)")
		} else {
			fmt.Println(s.ActiveDesk)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(currentCmd)
}
