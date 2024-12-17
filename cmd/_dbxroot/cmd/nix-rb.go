package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var rbCmd = &cobra.Command{
	Use:   "rb",
	Short: "Executes nixos-rebuild boot",
	Run: func(cmd *cobra.Command, args []string) {
		execCmd := exec.Command("nixos-rebuild", "boot", "-I", "nixos-config=/etc/nixos/configuration.nix")
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		err := execCmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild boot: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	nixCmd.AddCommand(rbCmd)
}
