package cmd

import (
	_ "embed"
	"log"
	"os"

	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/dogeorg/dogeboxd/pkg/system/lifecycle"
	"github.com/spf13/cobra"
)

var enterRecoveryModeCmd = &cobra.Command{
	Use:   "enter-recovery-mode",
	Short: "Run this to enter recovery mode. Your device will reboot.",
	Run: func(cmd *cobra.Command, args []string) {
		dataDir, err := cmd.Flags().GetString("dataDir")
		if err != nil {
			log.Println("Failed to get dataDir flag.")
			os.Exit(1)
		}

		hasFile := system.HasForceRecoveryFile(dataDir)

		if hasFile {
			log.Println("Will already enter recovery mode next boot.")
			os.Exit(0)
		}

		if err := system.ForceRecoveryNextBoot(dataDir); err != nil {
			log.Println("Failed to write file.")
			os.Exit(1)
		}

		log.Println("Wrote flag, rebooting..")

		lifecycleManager := lifecycle.NewLifecycleManager()
		lifecycleManager.Reboot()
	},
}

func init() {
	enterRecoveryModeCmd.Flags().StringP("dataDir", "d", "", "dogebox data dir")
	rootCmd.AddCommand(enterRecoveryModeCmd)
}
