package cmd

import (
	"fmt"
	"os"

	"github.com/Dogebox-WG/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var nixRBSetRelease string
var nixRBFlakeDir string
var nixRBGitHubToken string

var rbCmd = &cobra.Command{
	Use:   "rb",
	Short: "Executes nixos-rebuild boot",
	Run: func(cmd *cobra.Command, args []string) {
		if err := utils.RunNixOSRebuild("boot", nixRBSetRelease, nixRBFlakeDir, nixRBGitHubToken); err != nil {
			fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild boot: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rbCmd.Flags().StringVarP(&nixRBSetRelease, "set-release", "s", "", "rebuild with specific release (used for upgrades)")
	rbCmd.Flags().StringVar(&nixRBFlakeDir, "flake-dir", "", "rebuild from a specific flake directory")
	rbCmd.Flags().StringVar(&nixRBGitHubToken, "github-token", "", "GitHub token (use for more API request allocation)")
	nixCmd.AddCommand(rbCmd)
}
