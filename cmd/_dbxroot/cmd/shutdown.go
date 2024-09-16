package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var shutdownCmd = &cobra.Command{
	Use:   "shutdown",
	Short: "Shutdown the host system",
	Run: func(cmd *cobra.Command, args []string) {
		shutdownCmd := exec.Command("poweroff")
		err := shutdownCmd.Run()
		if err != nil {
			fmt.Printf("Error shutting down the system: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	shutdownCmd.AddCommand(rebootCmd)
}
