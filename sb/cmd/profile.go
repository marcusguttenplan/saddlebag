package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/desk"
)

var profileCmd = &cobra.Command{
	Use:   "profile",
	Short: "Print the active AWS profile",
	RunE: func(cmd *cobra.Command, args []string) error {
		deskID, _, _ := resolveCurrentDesk()
		if deskID == "" {
			fmt.Println("(none)")
			return nil
		}

		desks, err := desk.LoadAll()
		if err != nil {
			fmt.Println("(none)")
			return nil
		}

		d, ok := desks[deskID]
		if !ok || d.AWS == nil || d.AWS.Profile == "" {
			fmt.Println("(none)")
			return nil
		}

		fmt.Println(d.AWS.Profile)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(profileCmd)
}
