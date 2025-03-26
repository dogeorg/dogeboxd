package system

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"

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

const (
	one_gigabyte            = 1024 * 1024 * 1024
	ten_gigabytes           = 10 * one_gigabyte
	three_hundred_gigabytes = 300 * one_gigabyte
)

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

		isSuitableInstallSize := device.Size.Int64 >= ten_gigabytes
		isSuitableStorageSize := device.Size.Int64 >= three_hundred_gigabytes

		isSuitableDevice := isOKDevice && device.Size.Int64 > 0
		isAlreadyUsed := isMounted || hasChildren

		// This block package only seems to return a single mount point.
		// So we need to check if we're mounted at either / or /nix/store
		// to "reliably" determine if this is our boot media.
		if device.MountPoint == "/" || device.MountPoint == "/nix/store" || device.MountPoint == "/nix/.ro-store" {
			disk.BootMedia = true
		}

		// Check if any of our children are mounted as boot.
		for _, child := range device.Children {
			if child.MountPoint == "/" || child.MountPoint == "/nix/store" || device.MountPoint == "/nix/.ro-store" {
				disk.BootMedia = true
			}
		}

		// Even for devices we don't class as "usable" for storage, if we're
		// booting off it, we need to let the user select it (ie. no external storage)
		isUsableStorageDevice := isSuitableDevice || disk.BootMedia

		disk.Suitability = dogeboxd.SystemDiskSuitability{
			Install: dogeboxd.SystemDiskSuitabilityEntry{
				Usable: isSuitableDevice,
				SizeOK: isSuitableInstallSize,
			},
			Storage: dogeboxd.SystemDiskSuitabilityEntry{
				Usable: isUsableStorageDevice,
				SizeOK: isSuitableStorageSize,
			},
			IsAlreadyUsed: isAlreadyUsed,
		}

		disks = append(disks, disk)
	}

	return disks, nil
}

func InitStorageDevice(dbxState dogeboxd.DogeboxState) (string, error) {
	if dbxState.StorageDevice == "" || dbxState.InitialState.HasFullyConfigured {
		return "", nil
	}

	cmd := exec.Command("sudo", "_dbxroot", "prepare-storage-device", "--print", "--disk", dbxState.StorageDevice, "--dbx-secret", DBXRootSecret)

	var out bytes.Buffer
	cmd.Stdout = io.MultiWriter(&out, os.Stdout)
	cmd.Stderr = io.MultiWriter(&out, os.Stderr)

	// Execute the command
	err := cmd.Run()
	if err != nil {
		return "", fmt.Errorf("failed to execute _dbxroot prepare-storage-device: %w", err)
	}

	output := out.String()

	lines := strings.Split(strings.TrimSpace(output), "\n")
	partitionName := ""
	if len(lines) > 0 {
		lastLine := lines[len(lines)-1]
		parts := strings.Split(lastLine, " ")
		if len(parts) > 0 {
			partitionName = parts[len(parts)-1]
		}
	}

	if partitionName == "" {
		return "", fmt.Errorf("failed to get partition name")
	}

	return partitionName, nil
}

func GetBuildType() (string, error) {
	buildType, err := os.ReadFile("/opt/build-type")
	if err != nil {
		if os.IsNotExist(err) {
			return "unknown", nil
		}
		return "", fmt.Errorf("failed to read build type: %w", err)
	}
	return strings.TrimSpace(string(buildType)), nil
}

func InstallToDisk(config dogeboxd.ServerConfig, dbxState dogeboxd.DogeboxState, name string, t dogeboxd.Dogeboxd) error {
	if config.DevMode {
		t.Changes <- dogeboxd.Change{
			ID:     "test-install",
			Type:   "recovery",
			Update: "Dev mode enabled, skipping installation. You probably do not want to do this. re-run without dev mode if you do.",
		}
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
		if disk.Name == name && disk.Suitability.Install.Usable {
			diskExists = true
			break
		}
	}

	if !diskExists {
		return fmt.Errorf("specified disk '%s' not found in list of possible install disks", name)
	}

	buildType, err := GetBuildType()
	if err != nil {
		log.Printf("Failed to get build type: %v", err)
		return err
	}

	log.Printf("Starting to install to disk %s", name)

	var installFn func(string, dogeboxd.Dogeboxd) error

	installFn = dbxrootInstallToDisk

	// For the T6, we need to write the root FS over the EMMC
	// with DD, as we need all the arm-specific bootloaders and such.
	if buildType == "nanopc-T6" {
		installFn = dbxrootDDToDisk
	}

	if err := installFn(name, t); err != nil {
		log.Printf("Failed to install to disk: %v", err)
		return err
	}

	log.Printf("Installation completed successfully")

	return nil
}

func dbxrootInstallToDisk(disk string, t dogeboxd.Dogeboxd) error {
	var out bytes.Buffer
	cmd := exec.Command("sudo", "_dbxroot", "install-to-disk", "--disk", disk, "--dbx-secret", DBXRootSecret)
	cmd.Stdout = io.MultiWriter(&out, os.Stdout)
	cmd.Stderr = io.MultiWriter(&out, os.Stderr)

	err := cmd.Run()
	if err != nil {
		return err
	}

	t.Changes <- dogeboxd.Change{
		ID:     "install-output",
		Type:   "recovery",
		Update: out.String(),
	}
	return nil
}

func dbxrootDDToDisk(toDisk string, t dogeboxd.Dogeboxd) error {
	var out bytes.Buffer
	cmd := exec.Command("sudo", "_dbxroot", "dd-to-disk", "--target-disk", toDisk, "--dbx-secret", DBXRootSecret)
	cmd.Stdout = io.MultiWriter(&out, os.Stdout)
	cmd.Stderr = io.MultiWriter(&out, os.Stderr)

	err := cmd.Run()
	if err != nil {
		return err
	}

	t.Changes <- dogeboxd.Change{
		ID:     "dd-output",
		Type:   "recovery",
		Update: out.String(),
	}
	return nil
}
