package cmd

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var ddToDiskCmd = &cobra.Command{
	Use:   "dd-to-disk",
	Short: "Install Dogebox to a disk.",
	Long: `Install Dogebox to a disk.
This command requires --target-disk and --dbx-secret flags.

Example:
  _dbxroot dd-to-disk --target-disk /dev/sdb --dbx-secret ?`,
	Run: func(cmd *cobra.Command, args []string) {
		targetDisk, _ := cmd.Flags().GetString("target-disk")
		dbxSecret, _ := cmd.Flags().GetString("dbx-secret")

		if dbxSecret != system.DBXRootSecret {
			log.Printf("Invalid dbx secret")
			os.Exit(1)
		}

		defer func() {
			if r := recover(); r != nil {
				log.Printf("Failed to install to disk: %v", r)
				os.Exit(1)
			}
		}()

		disks, err := system.GetSystemDisks()
		if err != nil {
			log.Printf("Failed to get system disks: %v", err)
			os.Exit(1)
		}

		// Ensure target disk exists in disks
		var targetDiskExists bool
		var bootMediaDisk dogeboxd.SystemDisk
		for _, disk := range disks {
			if disk.Name == targetDisk {
				targetDiskExists = true
			}
			if disk.BootMedia && bootMediaDisk.Name == "" {
				bootMediaDisk = disk
			}
		}

		if !targetDiskExists {
			log.Printf("Target disk %s not found in system disks", targetDisk)
			os.Exit(1)
		}

		if bootMediaDisk.Name == "" {
			log.Printf("No boot media disk found")
			os.Exit(1)
		}

		if bootMediaDisk.Name == targetDisk {
			log.Printf("Source and target disks are the same: %s", targetDisk)
			os.Exit(1)
		}

		log.Printf("Using %s as source boot media", bootMediaDisk)
		log.Printf("Installing to target disk: %s", targetDisk)

		// Copy first 5gb of the disk. This _should_ contain all the bootloaders and enough of the rootfs.
		megaBytesToCopy := 5000

		utils.RunCommand("sudo", "dd", "if="+bootMediaDisk.Name, "of="+targetDisk, "bs=1M", "status=progress", "count="+fmt.Sprintf("%d", megaBytesToCopy))

		// Once we've copied the data, we need to find the new partition, mount it, and set/remove some flags.
		rootPartition, err := getWrittenRootPartition(targetDisk)
		if err != nil {
			log.Printf("Failed to get written root partition: %v", err)
			os.Exit(1)
		}

		utils.RunCommand("sudo", "mount", rootPartition, "/mnt")

		// Set an installed flag so we know not to try again.
		utils.RunCommand("sudo", "touch", "/mnt/opt/dbx-installed")

		// Remove the ro-media flag so we don't force the user to re-install.
		utils.RunCommand("sudo", "rm", "-rf", "/mnt/opt/ro-media")

		utils.RunCommand("sudo", "umount", "/mnt")

		log.Println("Finished installing. Please remove installation media and reboot.")
	},
}

func init() {
	rootCmd.AddCommand(ddToDiskCmd)

	ddToDiskCmd.Flags().StringP("target-disk", "d", "", "Disk to install to (required)")
	ddToDiskCmd.MarkFlagRequired("target-disk")

	ddToDiskCmd.Flags().StringP("dbx-secret", "s", "", "?")
	ddToDiskCmd.MarkFlagRequired("dbx-secret")
}

func getWrittenRootPartition(disk string) (string, error) {
	cmd := exec.Command("lsblk", disk, "-o", "name,label", "--json")
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to run lsblk command: %w", err)
	}

	var result struct {
		Blockdevices []struct {
			Name     string `json:"name"`
			Label    string `json:"label"`
			Children []struct {
				Name  string `json:"name"`
				Label string `json:"label"`
			} `json:"children,omitempty"`
		} `json:"blockdevices"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return "", fmt.Errorf("failed to parse lsblk output: %w", err)
	}

	for _, device := range result.Blockdevices {
		if device.Label == "nixos" {
			return "/dev/" + device.Name, nil
		}
		for _, child := range device.Children {
			if child.Label == "nixos" {
				return "/dev/" + child.Name, nil
			}
		}
	}

	return "", fmt.Errorf("no partition with label 'nixos' found")
}
