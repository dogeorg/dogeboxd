package cmd

import (
	_ "embed"
	"fmt"
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
			size := prettyPrintDiskSize(disk.Size.Int64)
			log.Printf(" - %s (%s)", disk.Name, size)
		}

		os.Exit(0)
	},
}

func init() {
	rootCmd.AddCommand(getDisksCmd)
}

func prettyPrintDiskSize(size int64) string {
	const (
		KB = 1024
		MB = 1024 * KB
		GB = 1024 * MB
		TB = 1024 * GB
	)

	switch {
	case size >= TB:
		return fmt.Sprintf("%.2f TB", float64(size)/float64(TB))
	case size >= GB:
		return fmt.Sprintf("%.2f GB", float64(size)/float64(GB))
	case size >= MB:
		return fmt.Sprintf("%.2f MB", float64(size)/float64(MB))
	case size >= KB:
		return fmt.Sprintf("%.2f KB", float64(size)/float64(KB))
	default:
		return fmt.Sprintf("%d B", size)
	}
}
