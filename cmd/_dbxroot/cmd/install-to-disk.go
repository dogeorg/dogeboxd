package cmd

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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
		runParted(disk, "mklabel", "gpt")
		runParted(disk, "mkpart", "root", "ext4", "512MB", "-8GB")
		runParted(disk, "mkpart", "swap", "linux-swap", "-8GB", "100%")
		runParted(disk, "mkpart", "ESP", "fat32", "1MB", "512MB")
		runParted(disk, "set", "3", "esp", "on")

		isNVME := strings.HasPrefix(disk, "/dev/nvme")
		partitionPrefix := ""

		if isNVME {
			partitionPrefix = "p"
		}

		rootPartition := fmt.Sprintf("%s%s1", disk, partitionPrefix)
		swapPartition := fmt.Sprintf("%s%s2", disk, partitionPrefix)
		espPartition := fmt.Sprintf("%s%s3", disk, partitionPrefix)

		// Format partitions
		runCommand("mkfs.ext4", "-L", "nixos", rootPartition)
		runCommand("mkswap", "-L", "swap", swapPartition)
		runCommand("mkfs.fat", "-F", "32", "-n", "boot", espPartition)

		// Ensure /mnt exists before we actually mount into it.
		if _, err := os.Stat("/mnt"); os.IsNotExist(err) {
			runCommand("mkdir", "/mnt")
		}

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

		// Install
		runCommand("nixos-install", "--no-root-passwd", "--root", "/mnt")

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

func runParted(device string, args ...string) error {
	args = append([]string{"parted", "-s", device, "--"}, args...)
	return runCommand(args...)
}

func runCommand(args ...string) error {
	log.Printf("----------------------------------------")
	log.Printf("Running command: %+v", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("Error running command: %v", err)
		panic(err)
	}
	log.Printf("----------------------------------------")
	return nil
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
