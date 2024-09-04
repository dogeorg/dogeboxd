package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var devPupCmd = &cobra.Command{
	Use:   "pup",
	Short: "Pup development commands",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("dev pup command called")
	},
}

func init() {
	devCmd.AddCommand(devPupCmd)
}
