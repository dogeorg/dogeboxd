package cmd

import (
	_ "embed"
	"log"
	"os"

	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

func exitBad(isSystemd bool) {
	if isSystemd {
		os.Exit(255)
		return
	}

	os.Exit(1)
}

var isRecoveryModeCmd = &cobra.Command{
	Use:   "is-recovery-mode",
	Short: "Check if the device is in recovery mode.",
	Run: func(cmd *cobra.Command, args []string) {
		dataDir, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			log.Println("Failed to get dataDir flag.")
			exitBad(true)
			return
		}

		systemd, err := cmd.Flags().GetBool("systemd")
		if err != nil {
			log.Println("Failed to get systemd flag.")
			exitBad((true))
			return
		}

		sm := system.NewStateManager(dataDir)
		err = sm.Load()
		if err != nil {
			log.Println("Failed to load state manager: ", err)
			exitBad(systemd)
			return
		}

		isInRecoveryMode := system.ShouldEnterRecovery(dataDir, sm)

		log.Println("Is in recovery mode:", isInRecoveryMode)

		if isInRecoveryMode {
			exitBad(systemd)
			return
		}

		os.Exit(0)
	},
}

func init() {
	isRecoveryModeCmd.Flags().StringP("data-dir", "d", "/opt/dogebox", "dogebox data dir")
	isRecoveryModeCmd.Flags().BoolP("systemd", "", false, "Exits with 255 instead of 1 if in recovery mode.")
	rootCmd.AddCommand(isRecoveryModeCmd)
}
