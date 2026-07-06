package cmd

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/Dogebox-WG/dogeboxd/cmd/_dbxroot/utils"
	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

type builderType string

const (
	builderIso  builderType = "iso"
	builderT6   builderType = "nanopc-t6"
	builderQemu builderType = "qemu"
)

// String is used both by fmt.Print and by Cobra in help text
func (e *builderType) String() string {
	return string(*e)
}

// Set must have pointer receiver so it doesn't change the value of a copy
func (e *builderType) Set(v string) error {
	switch v {
	case "iso", "nanopc-t6", "qemu":
		*e = builderType(v)
		return nil
	default:
		return errors.New(`must be one of "iso", "nanopc-t6", or "qemu"`)
	}
}

// Type is only used in help text
func (e *builderType) Type() string {
	return "builderType"
}

var installToDiskCmd = &cobra.Command{
	Use:   "install-to-disk",
	Short: "Install Dogebox to a disk.",
	Long: `Install Dogebox to a disk.
Example:
  _dbxroot install-to-disk --variant iso --disk /dev/sdb --dbx-secret ?

     --variant    -t install variant to use. "iso", "nanopc-t6", "qemu"
	 --disk       -d target disk for install
	 --dbx-secret -s dbx secret `,
	Run: func(cmd *cobra.Command, args []string) {
		variant := builderIso
		disk, _ := cmd.Flags().GetString("disk")
		dbxSecret, _ := cmd.Flags().GetString("dbx-secret")
		variantString, _ := cmd.Flags().GetString("variant")
		err := (&variant).Set(variantString)
		if err != nil {
			log.Printf("failed to parse variant: %v\n%v\n", variantString, err)
			os.Exit(1)
		}

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

		// Track whether the requested disk exists at all, and whether it is a
		// valid install target under the shared install-target policy.
		var targetDiskExists bool
		var targetDiskIsInstallTarget bool
		var bootMediaDisk dogeboxd.SystemDisk
		for _, d := range disks {
			if d.Name == disk {
				targetDiskExists = true
				targetDiskIsInstallTarget = system.IsInstallTargetDisk(d)
			}
			if d.BootMedia && bootMediaDisk.Name == "" {
				bootMediaDisk = d
			}
		}

		if !targetDiskExists {
			log.Printf("Target disk %s not found in system disks", disk)
			os.Exit(1)
		}
		if !targetDiskIsInstallTarget {
			log.Printf("Target disk %s is not a valid install target", disk)
			os.Exit(1)
		}

		if bootMediaDisk.Name == "" {
			log.Printf("No boot media disk found")
			os.Exit(1)
		}

		if bootMediaDisk.Name == disk {
			log.Printf("Source and target disks are the same: %s", disk)
			os.Exit(1)
		}

		log.Printf("Using %s as source boot media", bootMediaDisk.Name)
		log.Printf("Installing to target disk: %s", disk)

		hasPartitionPrefix := strings.HasPrefix(disk, "/dev/nvme") || strings.HasPrefix(disk, "/dev/mmcblk")
		partitionPrefix := ""

		if hasPartitionPrefix {
			partitionPrefix = "p"
		}

		// Ensure /mnt exists before we actually mount into it.
		utils.RunCommand("mkdir", "-p", "/mnt")

		if variant == builderT6 {
			create_t6_boot(disk, bootMediaDisk, partitionPrefix)
		} else {
			create_normal_boot(disk, partitionPrefix)
		}

		// Copy our NixOS configuration over
		utils.RunCommand("mkdir", "-p", "/mnt/etc/nixos/")
		utils.CopyFiles("/etc/nixos/", "/mnt/etc/nixos/")

		// Generate hardware-configuration.nix
		utils.RunCommand("nixos-generate-config", "--root", "/mnt")

		// Set an installed flag so we know not to try again.
		utils.RunCommand("mkdir", "-p", "/mnt/opt/")
		utils.RunCommand("touch", "/mnt/opt/dbx-installed")
		utils.RunCommand("chown", "dogeboxd:dogebox", "/mnt/opt/dbx-installed")

		systemClosure := utils.RunCommand("readlink", "-f", "/run/current-system")

		// Install
		utils.RunCommand("nixos-install", "--system", strings.TrimSpace(systemClosure), "--no-root-passwd", "--root", "/mnt")
		// TODO (ando - 28/07/2025) - Figure out if we need umount here, since
		//                            it did break iso install with `target busy`
		if variant != builderIso {
			utils.RunCommand("umount", "/mnt")
		}

		log.Println("Finished installing. Please remove installation media and reboot.")
	},
}

func init() {
	rootCmd.AddCommand(installToDiskCmd)

	installToDiskCmd.Flags().StringP("disk", "d", "", "Disk to install to (required)")
	installToDiskCmd.MarkFlagRequired("disk")

	installToDiskCmd.Flags().StringP("dbx-secret", "s", "", "?")
	installToDiskCmd.MarkFlagRequired("dbx-secret")

	installToDiskCmd.Flags().StringP("variant", "t", "", "Install variant to use. One of: \"iso\", \"nanopc-t6\", \"qemu\"")
	installToDiskCmd.MarkFlagRequired("variant")
}

func create_t6_boot(disk string, bootMediaDisk dogeboxd.SystemDisk, partitionPrefix string) {
	// Create partition table
	utils.RunParted(disk, "mklabel", "gpt")

	utils.RunParted(disk, "mkpart", "uboot", "16384s", "24575s")
	utils.RunParted(disk, "type", "1", "F808D051-1602-4DCD-9452-F9637FEFC49A")

	utils.RunParted(disk, "mkpart", "misc", "24576s", "32767s")
	utils.RunParted(disk, "type", "2", "C6D08308-E418-4124-8890-F8411E3D8D87")

	utils.RunParted(disk, "mkpart", "dtbo", "32768s", "40959s")
	utils.RunParted(disk, "type", "3", "2A583E58-486A-4BD4-ACE4-8D5454E97F5C")

	utils.RunParted(disk, "mkpart", "resource", "40960s", "73727s")
	utils.RunParted(disk, "type", "4", "6115F139-4F47-4BAF-8D23-B6957EAEE4B3")

	utils.RunParted(disk, "mkpart", "kernel", "73728s", "155647s")
	utils.RunParted(disk, "type", "5", "A83FBA16-D354-45C5-8B44-3EC50832D363")

	utils.RunParted(disk, "mkpart", "boot", "155648s", "221183s")
	utils.RunParted(disk, "type", "6", "500E2214-B72D-4CC3-D7C1-8419260130F5")

	utils.RunParted(disk, "mkpart", "recovery", "221184s", "286719s")
	utils.RunParted(disk, "type", "7", "E099DA71-5450-44EA-AA9F-1B771C582805")

	utils.RunParted(disk, "mkpart", "rootfs", "286720s", "100%")
	utils.RunParted(disk, "type", "8", "AF12D156-5D5B-4EE3-B415-8D492CA12EA9")
	utils.RunParted(disk, "set", "8", "boot", "on")
	utils.RunParted(disk, "set", "8", "legacy_boot", "on")

	// Raw copy idbloader from boot media to target disk. idbloader sits between the end of the partition table and the start of the first partition.
	utils.RunCommand("dd", "if=/etc/uboot/idbloader.img", "of="+disk, "bs=512", "seek=64", "iflag=fullblock", "conv=notrunc,fsync", "status=progress")

	// Raw copy u-boot from boot media partition 1 to target disk partition 1
	utils.RunCommand("dd", "if=/etc/uboot/u-boot.itb", "of="+fmt.Sprintf("%s%s1", disk, partitionPrefix), "status=progress")

	rootPartition := fmt.Sprintf("%s%s8", disk, partitionPrefix)

	utils.RunCommand("mkfs.ext4", "-L", "nixos", rootPartition)

	utils.RunCommand("mount", rootPartition, "/mnt")
}

func create_normal_boot(disk string, partitionPrefix string) {
	// Create partition table
	utils.RunParted(disk, "mklabel", "gpt")
	utils.RunParted(disk, "mkpart", "root", "ext4", "512MB", "-8GB")
	utils.RunParted(disk, "mkpart", "swap", "linux-swap", "-8GB", "100%")
	utils.RunParted(disk, "mkpart", "ESP", "fat32", "1MB", "512MB")
	utils.RunParted(disk, "set", "3", "esp", "on")

	rootPartition := fmt.Sprintf("%s%s1", disk, partitionPrefix)
	swapPartition := fmt.Sprintf("%s%s2", disk, partitionPrefix)
	espPartition := fmt.Sprintf("%s%s3", disk, partitionPrefix)

	// Format partitions
	utils.RunCommand("mkfs.ext4", "-L", "nixos", rootPartition)
	utils.RunCommand("mkswap", "-L", "swap", swapPartition)
	utils.RunCommand("mkfs.fat", "-F", "32", "-n", "boot", espPartition)

	// Mount everything up
	utils.RunCommand("mount", rootPartition, "/mnt")
	utils.RunCommand("mkdir", "-p", "/mnt/boot")
	utils.RunCommand("mount", "-o", "umask=077", espPartition, "/mnt/boot")
	utils.RunCommand("swapon", swapPartition)
}
