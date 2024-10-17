package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var prepareStorageDeviceCmd = &cobra.Command{
	Use:   "prepare-storage-device",
	Short: "Prepare a storage device for use with Dogebox.",
	Long: `Prepare a storage device for use with Dogebox.
This command requires --disk and --dbx-secret flags.

Example:
  _dbxroot prepare-storage-device --disk /dev/sdb --dbx-secret ?`,
	Run: func(cmd *cobra.Command, args []string) {
		disk, _ := cmd.Flags().GetString("disk")
		dbxSecret, _ := cmd.Flags().GetString("dbx-secret")
		print, _ := cmd.Flags().GetBool("print")

		if dbxSecret != system.DBXRootSecret {
			log.Printf("Invalid dbx secret")
			os.Exit(1)
		}

		defer func() {
			if r := recover(); r != nil {
				log.Printf("Failed to prepare storage device: %v", r)
				os.Exit(1)
			}
		}()

		utils.RunParted(disk, "mklabel", "gpt")
		utils.RunParted(disk, "mkpart", "root", "ext4", "0%", "100%")

		hasPartitionPrefix := strings.HasPrefix(disk, "/dev/nvme") || strings.HasPrefix(disk, "/dev/mmcblk")
		partitionPrefix := ""

		if strings.HasPrefix(disk, "/dev/loop") {
			// Loop device. This is probably only used for development, but I guess support it anyway?
			// We need to unmount, then remount it with partition scanning so it shows up again.
			backingFile, err := utils.GetLoopDeviceBackingFile(disk)
			if err != nil {
				log.Printf("Error getting loop device backing file: %v", err)
				os.Exit(1)
			}

			// Unmount it.
			utils.RunCommand("sudo", "losetup", "-d", disk)

			// Remount it with partition scanning.
			utils.RunCommand("sudo", "losetup", "-P", disk, backingFile)

			hasPartitionPrefix = true
		}

		if hasPartitionPrefix {
			partitionPrefix = "p"
		}

		partition := fmt.Sprintf("%s%s1", disk, partitionPrefix)
		utils.RunCommand("mkfs.ext4", "-L", "dogebox-storage", partition)

		log.Println("Finished preparing storage device.")

		if print {
			log.Printf(partition)
		}
	},
}

func init() {
	rootCmd.AddCommand(prepareStorageDeviceCmd)

	prepareStorageDeviceCmd.Flags().StringP("disk", "d", "", "Disk to format & prepare")
	prepareStorageDeviceCmd.MarkFlagRequired("disk")

	prepareStorageDeviceCmd.Flags().StringP("dbx-secret", "s", "", "?")
	prepareStorageDeviceCmd.MarkFlagRequired("dbx-secret")

	prepareStorageDeviceCmd.Flags().BoolP("print", "p", false, "Prints the resulting partition location")
}
