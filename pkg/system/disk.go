package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/dell/csi-baremetal/pkg/base/linuxutils/lsblk"
	"github.com/sirupsen/logrus"
)

const DBXRootSecret = "lPqz83ZMJjmaQDTa69uTshoDeL44wCbr"

func GetPossibleInstallDisks() ([]lsblk.BlockDevice, error) {
	lsb := lsblk.NewLSBLK(logrus.New())

	devices, err := lsb.GetBlockDevices("")
	if err != nil {
		return []lsblk.BlockDevice{}, err
	}

	possibleDisks := []lsblk.BlockDevice{}

	for _, device := range devices {
		// Ignore anything that's not a disk.
		if device.Type != "disk" {
			continue
		}

		// Ignore anything that's mounted.
		if device.MountPoint != "" {
			continue
		}

		// Ignore anything that is less than 100GB.
		if device.Size.Int64 < 100*1024*1024*1024 {
			continue
		}

		possibleDisks = append(possibleDisks, device)
	}

	return possibleDisks, nil
}

func InstallToDisk(name string) error {
	possibleDisks, err := GetPossibleInstallDisks()
	if err != nil {
		return err
	}

	// Check if the specified disk name exists in possibleDisks
	diskExists := false
	for _, disk := range possibleDisks {
		if disk.Name == name {
			diskExists = true
			break
		}
	}

	if !diskExists {
		return fmt.Errorf("specified disk '%s' not found in list of possible install disks", name)
	}

	log.Printf("Starting to install to disk %s", name)

	cmd := exec.Command("_dbxroot", "install-to-disk", "--disk", name, "--dbx-secret", DBXRootSecret)

	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	// Execute the command
	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("failed to execute _dbxroot install-to-disk: %w", err)
	}

	log.Printf("Installation completed successfully")

	return nil
}
