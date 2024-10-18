package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var installToDiskCmd = &cobra.Command{
	Use:   "install-to-disk",
	Short: "Install Dogebox to a disk.",
	Long: `Install Dogebox to a disk.
This command requires --disk and --dbx-secret flags.

Example:
  _dbxroot install-to-disk --disk /dev/sdb --dbx-secret ?`,
	Run: func(cmd *cobra.Command, args []string) {
		disk, _ := cmd.Flags().GetString("disk")
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

		// Create partition table
		utils.RunParted(disk, "mklabel", "gpt")
		utils.RunParted(disk, "mkpart", "root", "ext4", "512MB", "-8GB")
		utils.RunParted(disk, "mkpart", "swap", "linux-swap", "-8GB", "100%")
		utils.RunParted(disk, "mkpart", "ESP", "fat32", "1MB", "512MB")
		utils.RunParted(disk, "set", "3", "esp", "on")

		hasPartitionPrefix := strings.HasPrefix(disk, "/dev/nvme") || strings.HasPrefix(disk, "/dev/mmcblk")
		partitionPrefix := ""

		if hasPartitionPrefix {
			partitionPrefix = "p"
		}

		rootPartition := fmt.Sprintf("%s%s1", disk, partitionPrefix)
		swapPartition := fmt.Sprintf("%s%s2", disk, partitionPrefix)
		espPartition := fmt.Sprintf("%s%s3", disk, partitionPrefix)

		// Format partitions
		utils.RunCommand("mkfs.ext4", "-L", "nixos", rootPartition)
		utils.RunCommand("mkswap", "-L", "swap", swapPartition)
		utils.RunCommand("mkfs.fat", "-F", "32", "-n", "boot", espPartition)

		// Ensure /mnt exists before we actually mount into it.
		if _, err := os.Stat("/mnt"); os.IsNotExist(err) {
			utils.RunCommand("mkdir", "/mnt")
		}

		// Mount everything up
		utils.RunCommand("mount", rootPartition, "/mnt")
		utils.RunCommand("mkdir", "-p", "/mnt/boot")
		utils.RunCommand("mount", "-o", "umask=077", espPartition, "/mnt/boot")
		utils.RunCommand("swapon", swapPartition)

		// Copy our NixOS configuration over
		utils.RunCommand("mkdir", "-p", "/mnt/etc/nixos/")
		copyFiles("/etc/nixos/", "/mnt/etc/nixos/")

		// Generate hardware-configuration.nix
		utils.RunCommand("nixos-generate-config", "--root", "/mnt")

		// Set an installed flag so we know not to try again.
		utils.RunCommand("mkdir", "-p", "/mnt/opt/")
		utils.RunCommand("touch", "/mnt/opt/dbx-installed")

		// Install
		utils.RunCommand("nixos-install", "--no-root-passwd", "--root", "/mnt")

		log.Println("Finished installing. Please remove installation media and reboot.")
	},
}

func init() {
	rootCmd.AddCommand(installToDiskCmd)

	installToDiskCmd.Flags().StringP("disk", "d", "", "Disk to install to (required)")
	installToDiskCmd.MarkFlagRequired("disk")

	installToDiskCmd.Flags().StringP("dbx-secret", "s", "", "?")
	installToDiskCmd.MarkFlagRequired("dbx-secret")
}

func copyFiles(source string, destination string) error {
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(destination, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}

		return os.Chmod(destPath, info.Mode())
	})

	return err
}
