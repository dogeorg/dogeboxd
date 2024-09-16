package cmd

import (
	"github.com/spf13/cobra"
)

var pupCmd = &cobra.Command{
	Use:   "pup",
	Short: "Used for interacting with pups",
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(pupCmd)
}
