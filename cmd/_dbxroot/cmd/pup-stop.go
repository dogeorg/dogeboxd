package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop a specific pup",
	Long: `Stop a specific pup by providing its ID.
This command requires a --pupId flag with an alphanumeric value.

Example:
  pup stop --pupId mypup123`,
	Run: func(cmd *cobra.Command, args []string) {
		pupId, _ := cmd.Flags().GetString("pupId")
		if !utils.IsAlphanumeric(pupId) {
			fmt.Println("Error: pupId must contain only alphanumeric characters")
			return
		}

		fmt.Printf("Stopping container with ID: %s\n", pupId)

		// We enforce the pup- prefix here to make sure that no bad-actor
		// can stop a non-pup container that is running on the system.
		machineId := fmt.Sprintf("pup-%s", pupId)

		machineCtlCmd := exec.Command("sudo", "machinectl", "stop", machineId)
		machineCtlCmd.Stdout = os.Stdout
		machineCtlCmd.Stderr = os.Stderr

		if err := machineCtlCmd.Run(); err != nil {
			fmt.Fprintln(os.Stderr, "Error executing machinectl stop:", err)
			os.Exit(1)
		}
	},
}

func init() {
	pupCmd.AddCommand(stopCmd)

	stopCmd.Flags().StringP("pupId", "p", "", "ID of the pup to stop (required, alphanumeric only)")
	stopCmd.MarkFlagRequired("pupId")
}
