package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

var rsCmd = &cobra.Command{
	Use:   "rs",
	Short: "Executes nixos-rebuild switch",
	Run: func(cmd *cobra.Command, args []string) {
		execCmd := exec.Command("nixos-rebuild", "switch", "-I", "nixos-config=/etc/nixos/configuration.nix")
		execCmd.Stdout = os.Stdout
		execCmd.Stderr = os.Stderr

		err := execCmd.Run()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild switch: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	nixCmd.AddCommand(rsCmd)
}
