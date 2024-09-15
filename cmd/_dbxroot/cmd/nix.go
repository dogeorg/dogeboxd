package cmd

import (
	"github.com/spf13/cobra"
)

var nixCmd = &cobra.Command{
	Use:   "nix",
	Short: "Interact with nix",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(nixCmd)
}
