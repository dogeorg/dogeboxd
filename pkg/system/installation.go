package system

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dell/csi-baremetal/pkg/base/linuxutils/lsblk"
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/utils"
	"github.com/sirupsen/logrus"
)

const DBXRootSecret = "yes-i-will-destroy-everything-on-this-disk"

const (
	one_gigabyte                    = 1024 * 1024 * 1024
	ten_gigabytes                   = 10 * one_gigabyte
	three_hundred_gigabytes         = 300 * one_gigabyte
	isReadOnlyInstallationMediaFile = "/opt/ro-media"
)

func logToWebSocket(t dogeboxd.Dogeboxd, message string) {
	log.Printf("logging to web socket: %s", message)
	t.Changes <- dogeboxd.Change{
		ID:     "recovery",
		Type:   "recovery",
		Update: message,
	}
}

func IsInstalled(t dogeboxd.Dogeboxd, config dogeboxd.ServerConfig, dbxState dogeboxd.DogeboxState) (bool, error) {
	logToWebSocket(t, "checking if Dogebox OS is already installed")
	return checkNixOSDisksForFile(t, config, "/opt/dbx-installed")
}

func GetInstallationMode(t dogeboxd.Dogeboxd, dbxState dogeboxd.DogeboxState) (dogeboxd.BootstrapInstallationMode, error) {
	// If we've been configured, no install for you.
	if dbxState.InitialState.HasFullyConfigured {
		return dogeboxd.BootstrapInstallationModeCannotInstall, nil
	}

	// Check if we're running on RO installation media. If so, must install.
	isReadOnlyInstallationMedia, err := isReadOnlyInstallationMedia(t, "")
	if err != nil {
		return "", fmt.Errorf("error checking for RO installation media: %v", err)
	}
	if isReadOnlyInstallationMedia {
		return dogeboxd.BootstrapInstallationModeMustInstall, nil
	}

	// Otherwise, the user can optionally install.
	return dogeboxd.BootstrapInstallationModeCanInstall, nil
}

func isReadOnlyInstallationMedia(t dogeboxd.Dogeboxd, mountPoint string) (bool, error) {
	roMediaPath := filepath.Join(mountPoint, isReadOnlyInstallationMediaFile)
	var isMedia bool
	if _, err := os.Stat(roMediaPath); err != nil {
		if !os.IsNotExist(err) {
			logToWebSocket(t, fmt.Sprintf("error checking installation media flag: %v", err))
			return false, err
		}
		isMedia = false
	} else {
		isMedia = true
	}
	logToWebSocket(t, fmt.Sprintf("mount point %s is installation media? %v", mountPoint, isMedia))
	return isMedia, nil
}

func mountAndCheckDiskForFile(t dogeboxd.Dogeboxd, config dogeboxd.ServerConfig, devicePath, targetFile string, ignoreInstallMedia bool) (bool, error) {
	// Create a temporary mount point
	mountPoint, err := os.MkdirTemp(config.TmpDir, "tmp-mount")
	if err != nil {
		return false, fmt.Errorf("failed to create temporary mount point: %v", err)
	}
	defer os.RemoveAll(mountPoint) // Clean up temp directory

	// Mount the device using the full path with sudo
	mountCmd := exec.Command("sudo", "mount", devicePath, mountPoint)
	logToWebSocket(t, fmt.Sprintf("mounting device %s to %s", devicePath, mountPoint))
	if err := mountCmd.Run(); err != nil {
		return false, fmt.Errorf("failed to mount %s: %v", devicePath, err)
	}
	defer func() {
		// Ensure unmount happens even if file check fails
		unmountCmd := exec.Command("sudo", "umount", mountPoint)
		if err := unmountCmd.Run(); err != nil {
			logToWebSocket(t, fmt.Sprintf("warning: failed to unmount %s: %v", mountPoint, err))
		}
	}()

	// If this is install media and therefore has a file at /opt/ro-media, return false
	if !ignoreInstallMedia {
		isMedia, err := isReadOnlyInstallationMedia(t, mountPoint)
		if err != nil {
			return false, err
		}
		if isMedia {
			logToWebSocket(t, fmt.Sprintf("This device is installation media. Not checking for file %s", targetFile))
			return false, nil
		}
	}

	// Check for the target file
	filePath := filepath.Join(mountPoint, targetFile)
	_, err = os.Stat(filePath)
	if os.IsNotExist(err) {
		return false, nil
	} else if err != nil {
		return false, fmt.Errorf("error checking file %s: %v", filePath, err)
	}
	return true, nil
}

func findNixOSDisks(t dogeboxd.Dogeboxd) ([]string, error) {
	disks, err := GetSystemDisks()
	if err != nil {
		logToWebSocket(t, fmt.Sprintf("error getting system disks: %v", err))
		return nil, err
	}

	//return a string of all the disks that have the label 'nixos'
	var nixosDisks []string
	for _, disk := range disks {
		// Check if the disk itself has the nixos label
		if disk.Label == "nixos" {
			nixosDisks = append(nixosDisks, disk.Path)
			continue
		}

		// Check if any of the disk's children have the nixos label
		for _, child := range disk.Children {
			if child.Label == "nixos" {
				nixosDisks = append(nixosDisks, child.Path)
			}
		}
	}
	return nixosDisks, nil
}

func checkNixOSDisksForFile(t dogeboxd.Dogeboxd, config dogeboxd.ServerConfig, targetFile string) (bool, error) {
	// Find NixOS labeled disks
	disks, err := findNixOSDisks(t)
	if err != nil {
		return false, err
	}
	logToWebSocket(t, fmt.Sprintf("found %d NixOS labeled disks", len(disks)))

	// Check each disk for the target file
	for _, disk := range disks {
		logToWebSocket(t, fmt.Sprintf("checking disk %s for file %s", disk, targetFile))
		exists, err := mountAndCheckDiskForFile(t, config, disk, targetFile, false)
		if err != nil {
			logToWebSocket(t, fmt.Sprintf("Error processing disk %s: %v", disk, err))
			continue
		}
		if exists {
			logToWebSocket(t, fmt.Sprintf("found target file %s on disk: %s", targetFile, disk))
			return true, nil
		}
	}
	logToWebSocket(t, fmt.Sprintf("did not find target file %s on any nixos disks", targetFile))
	return false, nil
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

		// Get label information using lsblk
		cmd := exec.Command("lsblk", device.Name, "-o", "name,label,path", "--json")
		output, err := cmd.Output()
		if err != nil {
			log.Printf("Warning: failed to get label for device %s: %v", device.Name, err)
		} else {
			var result struct {
				Blockdevices []struct {
					Name     string `json:"name"`
					Label    string `json:"label"`
					Path     string `json:"path"`
					Children []struct {
						Name  string `json:"name"`
						Label string `json:"label"`
						Path  string `json:"path"`
					} `json:"children,omitempty"`
				} `json:"blockdevices"`
			}

			if err := json.Unmarshal(output, &result); err != nil {
				log.Printf("Warning: failed to parse lsblk output for device %s: %v", device.Name, err)
			} else if len(result.Blockdevices) > 0 {
				disk.Label = result.Blockdevices[0].Label
				disk.Path = result.Blockdevices[0].Path

				// Convert children to SystemDisk format
				for _, child := range result.Blockdevices[0].Children {
					disk.Children = append(disk.Children, dogeboxd.SystemDisk{
						Name:  child.Name,
						Label: child.Label,
						Path:  child.Path,
					})
				}
			}
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

func InstallToDisk(t dogeboxd.Dogeboxd, config dogeboxd.ServerConfig, dbxState dogeboxd.DogeboxState, name string) error {
	t.Changes <- dogeboxd.Change{
		ID:     "install-output",
		Type:   "recovery",
		Update: "Install to disk started",
	}
	if config.DevMode {
		t.Changes <- dogeboxd.Change{
			ID:     "warning",
			Type:   "recovery",
			Update: "Dev mode enabled, skipping installation. You probably do not want to do this. re-run without dev mode if you do.",
		}
		return nil
	}

	if !config.Recovery {
		return fmt.Errorf("installation can only be done in recovery mode")
	}

	installMode, err := GetInstallationMode(t, dbxState)
	if err != nil {
		return err
	}

	if installMode != dogeboxd.BootstrapInstallationModeMustInstall && installMode != dogeboxd.BootstrapInstallationModeCanInstall {
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

type lineStreamWriter struct {
	t        dogeboxd.Dogeboxd
	changeID string
	buf      []byte
}

func newLineStreamWriter(t dogeboxd.Dogeboxd, changeID string) *lineStreamWriter {
	return &lineStreamWriter{
		t:        t,
		changeID: changeID,
		buf:      make([]byte, 0),
	}
}

func (w *lineStreamWriter) Write(p []byte) (n int, err error) {
	for _, b := range p {
		if b == '\n' || b == '\r' {
			if len(w.buf) > 0 {
				w.t.Changes <- dogeboxd.Change{
					ID:     w.changeID,
					Type:   "recovery",
					Update: string(w.buf),
				}
				w.buf = w.buf[:0]
			}
		} else {
			w.buf = append(w.buf, b)
		}
	}
	return len(p), nil
}

func dbxrootInstallToDisk(disk string, t dogeboxd.Dogeboxd) error {
	cmd := exec.Command("sudo", "_dbxroot", "install-to-disk", "--disk", disk, "--dbx-secret", DBXRootSecret)
	cmd.Stdout = newLineStreamWriter(t, "install-output")
	cmd.Stderr = newLineStreamWriter(t, "install-output")

	return cmd.Run()
}

func dbxrootDDToDisk(toDisk string, t dogeboxd.Dogeboxd) error {
	cmd := exec.Command("sudo", "_dbxroot", "dd-to-disk", "--target-disk", toDisk, "--dbx-secret", DBXRootSecret)
	cmd.Stdout = newLineStreamWriter(t, "dd-output")
	cmd.Stderr = newLineStreamWriter(t, "dd-output")

	return cmd.Run()
}
