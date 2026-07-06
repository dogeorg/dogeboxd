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
	"regexp"
	"slices"
	"strings"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/utils"
	"github.com/dell/csi-baremetal/pkg/base/linuxutils/lsblk"
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

// Dogebox OS installation state based on boot drive type is either:
// From Installation Media (Read-Only mode):
// - No Installed OS - BootstrapInstallationStateNotInstalled
// - Installed, Unconfigured OS - BootstrapInstallationStateUnconfigured
// - Installed, Configured OS - BootstrapInstallationStateConfigured
// From Installed Location (not Read-Only mode):
// - Installed, Unconfigured - BootstrapInstallationStateUnconfigured
// - Installed, Configured - BootstrapInstallationStateConfigured
func GetInstallationState(t dogeboxd.Dogeboxd, config dogeboxd.ServerConfig, dbxState dogeboxd.DogeboxState) (dogeboxd.BootstrapInstallationBootMedia, dogeboxd.BootstrapInstallationState, error) {
	bootMedia := dogeboxd.BootstrapInstallationMediaReadWrite
	isReadOnlyInstallationMedia, err := isReadOnlyInstallationMedia(t, "")
	if err != nil {
		return bootMedia, "", fmt.Errorf("error checking for RO installation media: %v", err)
	}
	if isReadOnlyInstallationMedia {
		bootMedia = dogeboxd.BootstrapInstallationMediaReadOnly
	}

	// If we've been configured, no install for you.
	if dbxState.InitialState.HasFullyConfigured {
		return bootMedia, dogeboxd.BootstrapInstallationStateConfigured, nil
	}

	isInstalled, err := IsInstalled(t, config, dbxState)
	if err != nil {
		log.Printf("Could not determine if system is installed: %v", err)
		isInstalled = false
	}

	if isInstalled {
		return bootMedia, dogeboxd.BootstrapInstallationStateUnconfigured, nil
	}

	return bootMedia, dogeboxd.BootstrapInstallationStateNotInstalled, nil
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
	logToWebSocket(t, fmt.Sprintf("mounting and checking disk for file %s", targetFile))

	// Create a temporary mount point
	mountPoint, err := os.MkdirTemp(config.TmpDir, "tmp-mount")
	if err != nil {
		return false, fmt.Errorf("failed to create temporary mount point: %v", err)
	}
	defer os.RemoveAll(mountPoint) // Clean up temp directory

	// Mount the device
	mountCmd := exec.Command("sudo", "_dbxroot", "mount-disk", devicePath, mountPoint)
	logToWebSocket(t, fmt.Sprintf("Attempting to mount device %s to %s with command: %s", devicePath, mountPoint, mountCmd.String()))

	if err := mountCmd.Run(); err != nil {
		return false, fmt.Errorf("failed to mount %s: %v", devicePath, err)
	}
	logToWebSocket(t, "Mount command executed successfully")

	defer func() {
		// Ensure unmount happens even if file check fails
		unmountCmd := exec.Command("sudo", "_dbxroot", "unmount-disk", mountPoint)
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
		cmd := exec.Command("lsblk", device.Name, "-o", "name,label,path,mountpoints", "--json")
		output, err := cmd.Output()
		if err != nil {
			log.Printf("Warning: failed to get label for device %s: %v", device.Name, err)
		} else {
			var result struct {
				Blockdevices []struct {
					Name        string   `json:"name"`
					Label       string   `json:"label"`
					Path        string   `json:"path"`
					Mountpoints []string `json:"mountpoints"`
					Children    []struct {
						Name        string   `json:"name"`
						Label       string   `json:"label"`
						Path        string   `json:"path"`
						Mountpoints []string `json:"mountpoints"`
					} `json:"children,omitempty"`
				} `json:"blockdevices"`
			}

			if err := json.Unmarshal(output, &result); err != nil {
				log.Printf("Warning: failed to parse lsblk output for device %s: %v", device.Name, err)
			} else if len(result.Blockdevices) > 0 {
				disk.Label = result.Blockdevices[0].Label
				disk.Path = result.Blockdevices[0].Path
				disk.MountPoints = result.Blockdevices[0].Mountpoints

				// Convert children to SystemDisk format
				for _, child := range result.Blockdevices[0].Children {
					disk.Children = append(disk.Children, dogeboxd.SystemDisk{
						Name:        child.Name,
						Label:       child.Label,
						Path:        child.Path,
						MountPoints: child.Mountpoints,
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
		disk.BootMedia = IsDiskNixRoot(disk)

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

func GetInstallTargetDisks() ([]dogeboxd.SystemDisk, error) {
	disks, err := GetSystemDisks()
	if err != nil {
		return []dogeboxd.SystemDisk{}, err
	}

	installDisks := make([]dogeboxd.SystemDisk, 0, len(disks))
	for _, disk := range disks {
		if IsInstallTargetDisk(disk) {
			installDisks = append(installDisks, disk)
		}
	}

	return installDisks, nil
}

func IsInstallTargetDisk(disk dogeboxd.SystemDisk) bool {
	if !disk.Suitability.Install.Usable || !disk.Suitability.Install.SizeOK {
		return false
	}

	// Never offer install targets that correspond to the device we are
	// currently running from.
	if disk.BootMedia {
		return false
	}

	return true
}

func IsDiskNixRoot(disk dogeboxd.SystemDisk) bool {
	nixRootMountPoints := []string{"/", "/nix/store", "/nix/.ro-store"}

	for _, mountPoint := range nixRootMountPoints {
		if slices.Contains(disk.MountPoints, mountPoint) {
			return true
		}
	}

	for _, child := range disk.Children {
		if IsDiskNixRoot(child) {
			return true
		}
	}
	return false
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
	re := regexp.MustCompile(`prepared partition: (.+)`)
	for i := len(lines) - 1; i >= 0; i-- {
		line := lines[i]
		submatch := re.FindSubmatch([]byte(line))
		if len(submatch) == 2 {
			partitionName = string(submatch[1])
			break
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

	disks, err := GetInstallTargetDisks()
	if err != nil {
		return err
	}

	// Check if the specified disk name exists in the filtered list of install target disks
	diskExists := false
	for _, disk := range disks {
		if disk.Name == name {
			diskExists = true
			break
		}
	}

	if !diskExists {
		return fmt.Errorf("specified disk '%s' not found in list of install target disks", name)
	}

	buildType, err := GetBuildType()
	if err != nil {
		log.Printf("Failed to get build type: %v", err)
		return err
	}

	log.Printf("Starting to install to disk %s", name)

	if err := dbxrootInstallToDisk(name, t, buildType); err != nil {
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

func dbxrootInstallToDisk(disk string, t dogeboxd.Dogeboxd, buildType string) error {
	cmd := exec.Command("sudo", "_dbxroot", "install-to-disk", "--variant", buildType, "--disk", disk, "--dbx-secret", DBXRootSecret)
	cmd.Stdout = newLineStreamWriter(t, "install-output")
	cmd.Stderr = newLineStreamWriter(t, "install-output")

	return cmd.Run()
}
