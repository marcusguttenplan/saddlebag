package cmd

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/marcusguttenplan/sb/internal/ipc"
)

var switchCmd = &cobra.Command{
	Use:   "switch <desk>",
	Short: "Switch to a different desk (context)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		deskName := args[0]

		resp, err := ipc.Send(context.Background(), ipc.Command{
			Action: "switch",
			Desk:   deskName,
		})
		if err != nil {
			return fmt.Errorf("switching desk: %w", err)
		}

		fmt.Printf("Switched to %s: %s\n", deskName, resp.Message)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(switchCmd)
}
