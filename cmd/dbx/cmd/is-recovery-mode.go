package cmd

import (
	_ "embed"
	"fmt"
	"log"
	"os"

	"github.com/dogeorg/dogeboxd/cmd/dbx/utils"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var isRecoveryModeCmd = &cobra.Command{
	Use:   "is-recovery-mode",
	Short: "Check if the device is in recovery mode.",
	Run: func(cmd *cobra.Command, args []string) {
		dataDir, err := cmd.Flags().GetString("data-dir")
		if err != nil {
			log.Println("Failed to get dataDir flag.")
			utils.ExitBad(true)
			return
		}

		systemd, err := cmd.Flags().GetBool("systemd")
		if err != nil {
			log.Println("Failed to get systemd flag.")
			utils.ExitBad(true)
			return
		}

		store, err := dogeboxd.NewStoreManager(fmt.Sprintf("%s/dogebox.db", dataDir))
		if err != nil {
			log.Println("couldn't open store-manager db", err)
			utils.ExitBad(systemd)
			return
		}
		sm := system.NewStateManager(store)

		isInRecoveryMode := system.IsRecoveryMode(dataDir, sm)

		log.Println("Is in recovery mode:", isInRecoveryMode)

		if isInRecoveryMode {
			utils.ExitBad(systemd)
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
