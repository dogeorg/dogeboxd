package cmd

import (
	"fmt"
	"log"
	"os"
	"os/exec"

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

		runParted(disk, "mklabel", "gpt")
		runParted(disk, "mkpart", "root", "ext4", "100%")

		partitionPrefix := ""

		if isNVME {
			partitionPrefix = "p"
		}

		partition := fmt.Sprintf("%s%s1", disk, partitionPrefix)
		runCommand("mkfs.ext4", "-L", "dogebox-storage", partition)

		log.Println("Finished preparing storage device.")
	},
}

func init() {
	rootCmd.AddCommand(prepareStorageDeviceCmd)

	prepareStorageDeviceCmd.Flags().StringP("disk", "d", "", "Disk to format & prepare")
	prepareStorageDeviceCmd.MarkFlagRequired("disk")

	prepareStorageDeviceCmd.Flags().StringP("dbx-secret", "s", "", "?")
	prepareStorageDeviceCmd.MarkFlagRequired("dbx-secret")
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
