package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var rebootCmd = &cobra.Command{
	Use:   "reboot",
	Short: "Reboot the host system",
	Run: func(cmd *cobra.Command, args []string) {
		rebootCmd := exec.Command("reboot")
		err := rebootCmd.Run()
		if err != nil {
			fmt.Printf("Error rebooting the system: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(rebootCmd)
}
