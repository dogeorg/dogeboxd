package cmd

import (
	_ "embed"
	"log"
	"os"

	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var getDisksCmd = &cobra.Command{
	Use:   "get-disks",
	Short: "Get a list of possible install disks.",
	Run: func(cmd *cobra.Command, args []string) {
		disks, err := system.GetPossibleInstallDisks()
		if err != nil {
			log.Printf("Failed to get possible install disks: %+v", err)
			os.Exit(1)
		}

		log.Println("Possible install disks:")

		for _, disk := range disks {
			log.Printf(" - %s (%s)", disk.Name, disk.SizePretty)
		}

		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(getDisksCmd)
}
