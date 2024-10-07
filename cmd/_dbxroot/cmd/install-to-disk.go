package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var installToDiskCmd = &cobra.Command{
	Use:   "install-to-disk",
	Short: "Install Dogebox to a disk.",
	Long: `Install Dogebox to a disk.
This command requires --disk and --dbx-secret flags.

Example:
  _dbxroot install-to-disk --disk /dev/sdb --dbx-secret :)`,
	Run: func(cmd *cobra.Command, args []string) {
		disk, _ := cmd.Flags().GetString("disk")
		dbxSecret, _ := cmd.Flags().GetString("dbx-secret")

		if dbxSecret != system.DBXRootSecret {
			log.Printf("Invalid dbx secret")
			os.Exit(1)
		}

		// Create partition table
		runParted(disk, "mklabel", "gpt")
		runParted(disk, "mkpart", "root", "ext4", "512MB", "-8GB")
		runParted(disk, "mkpart", "swap", "linux-swap", "-8GB", "100%")
		runParted(disk, "mkpart", "ESP", "fat32", "1MB", "512MB")
		runParted(disk, "set", "3", "esp", "on")

		rootPartition := fmt.Sprintf("%s1", disk)
		swapPartition := fmt.Sprintf("%s2", disk)
		espPartition := fmt.Sprintf("%s3", disk)

		// Format partitions
		runCommand("mkfs.ext4", "-L", "nixos", rootPartition)
		runCommand("mkswap", "-L", "swap", swapPartition)
		runCommand("mkfs.fat", "-F", "32", "-n", "boot", espPartition)

		// Mount everything up
		runCommand("mount", rootPartition, "/mnt")
		runCommand("mkdir", "-p", "/mnt/boot")
		runCommand("mount", "-o", "umask=077", espPartition, "/mnt/boot")
		runCommand("swapon", swapPartition)

		// Copy our NixOS configuration over
		runCommand("mkdir", "-p", "/mnt/etc/nixos/")
		copyFiles("/etc/nixos/", "/mnt/etc/nixos/")

		// Generate hardware-configuration.nix
		runCommand("nixos-generate-config", "--root", "/mnt")

		// TODO: somehow inject the EFI bootloader & variable modification stuff into /mnt/etc/nixos/configuration.nix
		// TODO: update dogebox root project to include hardware-configuration.nix if it exists?

		// Install
		runCommand("nixos-install", "--no-root-passwd", "--root", "/mnt")
		runCommand("umount", "/mnt")
	},
}

func init() {
	pupCmd.AddCommand(installToDiskCmd)

	installToDiskCmd.Flags().StringP("disk", "d", "", "Disk to install to (required)")
	installToDiskCmd.MarkFlagRequired("disk")

	installToDiskCmd.Flags().StringP("dbx-secret", "s", "", "")
	installToDiskCmd.MarkFlagRequired("dbx-secret")
}

func runParted(device string, args ...string) error {
	log.Printf("Running parted: %s -- %+v", device, args)
	parted := exec.Command("parted", append([]string{device, "--"}, args...)...)
	parted.Stdout = os.Stdout
	parted.Stderr = os.Stderr
	return parted.Run()
}

func runCommand(args ...string) error {
	log.Printf("Running command: %+v", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
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
