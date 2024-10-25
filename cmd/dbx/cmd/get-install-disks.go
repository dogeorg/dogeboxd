package cmd

import (
	_ "embed"
	"log"
	"os"

	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var getDisksCmd = &cobra.Command{
	Use:   "get-install-disks",
	Short: "Get a list of suitable disks you can install DogeboxOS to.",
	Run: func(cmd *cobra.Command, args []string) {
		disks, err := system.GetSystemDisks()
		if err != nil {
			log.Printf("Failed to get list of disks: %+v", err)
			os.Exit(1)
		}

		if len(disks) == 0 {
			log.Println("No suitable install disks found.")
			os.Exit(1)
		}

		log.Println("Suitable install disks:")

		for _, disk := range disks {
			if !disk.Suitability.Install.Usable {
				continue
			}

			log.Printf(" - %s (%s)", disk.Name, disk.SizePretty)
		}

		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(getDisksCmd)
}
