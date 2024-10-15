package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/dell/csi-baremetal/pkg/base/linuxutils/lsblk"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/utils"
	"github.com/sirupsen/logrus"
)

const DBXRootSecret = "yes-i-will-destroy-everything-on-this-disk"

func GetInstallationMode(dbxState dogeboxd.DogeboxState) (dogeboxd.BootstrapInstallationMode, error) {
	// First, check if we're already installed.
	if _, err := os.Stat("/opt/dbx-installed"); err == nil {
		return dogeboxd.BootstrapInstallationModeIsInstalled, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("error checking for RO installation media: %v", err)
	}

	// If we're not already installed, but we've been configured, no install for you.
	if dbxState.InitialState.HasFullyConfigured {
		return dogeboxd.BootstrapInstallationModeCannotInstall, nil
	}

	// Check if we're running on RO installation media. If so, must install.
	if _, err := os.Stat("/opt/ro-media"); err == nil {
		return dogeboxd.BootstrapInstallationModeMustInstall, nil
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("error checking for RO installation media: %v", err)
	}

	// Otherwise, the user can optionally install.
	return dogeboxd.BootstrapInstallationModeCanInstalled, nil
}

func GetSystemDisks() ([]dogeboxd.SystemDisk, error) {
	lsb := lsblk.NewLSBLK(logrus.New())

	devices, err := lsb.GetBlockDevices("")
	if err != nil {
		return []dogeboxd.SystemDisk{}, err
	}

	disks := []dogeboxd.SystemDisk{}

	for _, device := range devices {
		disk := dogeboxd.SystemDisk{
			Name:       device.Name,
			Size:       device.Size.Int64,
			SizePretty: utils.PrettyPrintDiskSize(device.Size.Int64),
		}

		// We will likely never see loop devices in the wild,
		// but it's useful to support these for development.
		isOKDevice := device.Type == "disk" || device.Type == "loop"

		isMounted := device.MountPoint != ""
		hasChildren := len(device.Children) > 0
		isZeroBytes := device.Size.Int64 == 0
		isOver10GB := device.Size.Int64 >= 10*1024*1024*1024
		isOver300GB := device.Size.Int64 >= 100*1024*1024*1024

		// Don't bother even returning these.
		if isZeroBytes {
			continue
		}

		if isOKDevice && !isMounted && !hasChildren {
			if isOver300GB {
				disk.SuitableDataDrive = true
			}

			if isOver10GB {
				disk.SuitableInstallDrive = true
			}
		}

		// This block package only seems to return a single mount point.
		// So we need to check if we're mounted at either / or /nix/store
		// to "reliably" determine if this is our boot media.
		if device.MountPoint == "/" || device.MountPoint == "/nix/store" {
			disk.BootMedia = true
		}

		// Check if any of our children are mounted as boot.
		for _, child := range device.Children {
			if child.MountPoint == "/" || child.MountPoint == "/nix/store" {
				disk.BootMedia = true
			}
		}

		disks = append(disks, disk)
	}

	return disks, nil
}

func InstallToDisk(config dogeboxd.ServerConfig, dbxState dogeboxd.DogeboxState, name string) error {
	if config.DevMode {
		log.Printf("Dev mode enabled, skipping installation. You probably do not want to do this. re-run without dev mode if you do.")
		return nil
	}

	if !config.Recovery {
		return fmt.Errorf("installation can only be done in recovery mode")
	}

	installMode, err := GetInstallationMode(dbxState)
	if err != nil {
		return err
	}

	if installMode != dogeboxd.BootstrapInstallationModeMustInstall && installMode != dogeboxd.BootstrapInstallationModeCanInstalled {
		return fmt.Errorf("installation is not possible with current system state: %s", installMode)
	}

	disks, err := GetSystemDisks()
	if err != nil {
		return err
	}

	// Check if the specified disk name exists in possibleDisks
	diskExists := false
	for _, disk := range disks {
		if disk.Name == name && disk.SuitableInstallDrive {
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
