package system

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var VALID_RECOVERY_FILES = []string{"RECOVERY", "RECOVERY.TXT"}

// This function should do detection on whether or not we should enter our "Recovery Mode".
// This can always be overriden by a CLI flag if necessary. This will not return true if the
// current instance of dogeboxd has booted in recovery. Use `IsRecoveryMode()` for that.
func ShouldEnterRecovery(dogeboxDataDir string, sm dogeboxd.StateManager) bool {
	return hasExternalRecoveryTXT() || isInitialConfiguration(sm) || HasForceRecoveryFile(dogeboxDataDir)
}

func IsRecoveryMode(dogeboxDataDir string, sm dogeboxd.StateManager) bool {
	if ShouldEnterRecovery(dogeboxDataDir, sm) {
		return true
	}

	bootedRecoveryPath := filepath.Join(dogeboxDataDir, "booted_recovery")
	if _, err := os.Stat(bootedRecoveryPath); err == nil {
		return true
	}
	return false
}

func DidEnterRecovery(dogeboxDataDir string) error {
	forceRecoveryPath := filepath.Join(dogeboxDataDir, "force_recovery_next_boot")
	if _, err := os.Stat(forceRecoveryPath); err == nil {
		if err := os.Remove(forceRecoveryPath); err != nil {
			return fmt.Errorf("failed to remove force_recovery_next_boot file: %w", err)
		}
	}

	bootedRecoveryPath := filepath.Join(dogeboxDataDir, "booted_recovery")
	file, err := os.Create(bootedRecoveryPath)
	if err != nil {
		return fmt.Errorf("failed to create booted_recovery file: %w", err)
	}
	defer file.Close()

	return nil
}

func ForceRecoveryNextBoot(dataDir string) error {
	filePath := filepath.Join(dataDir, "force_recovery_next_boot")
	file, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("failed to create force recovery file: %w", err)
	}
	defer file.Close()
	return nil
}

func UnforceRecoveryNextBoot(dataDir string) error {
	filePath := filepath.Join(dataDir, "force_recovery_next_boot")
	if _, err := os.Stat(filePath); err == nil {
		if err := os.Remove(filePath); err != nil {
			return fmt.Errorf("failed to remove force recovery file: %w", err)
		}
	}

	bootedRecoveryPath := filepath.Join(dataDir, "booted_recovery")
	if _, err := os.Stat(bootedRecoveryPath); err == nil {
		if err := os.Remove(bootedRecoveryPath); err != nil {
			return fmt.Errorf("failed to remove booted_recovery file: %w", err)
		}
	}

	return nil
}

func HasForceRecoveryFile(dataDir string) bool {
	filePath := filepath.Join(dataDir, "force_recovery_next_boot")
	if _, err := os.Stat(filePath); err == nil {
		return true
	}
	return false
}

func isInitialConfiguration(sm dogeboxd.StateManager) bool {
	completedInitialConfiguration := sm.Get().Dogebox.InitialState.HasFullyConfigured

	if !completedInitialConfiguration {
		log.Println("Dogebox has not completed initial configuration, forcing recovery mode..")
	}

	return !completedInitialConfiguration
}

func hasExternalRecoveryTXT() bool {
	file, err := os.Open("/proc/mounts")
	if err != nil {
		fmt.Printf("Error opening /proc/mounts: %v\n", err)
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)

		// Ensure the line has enough fields (device, mount point, etc.)
		if len(fields) < 2 {
			continue
		}

		device := fields[0]
		mountPoint := fields[1]

		// Check if the device name starts with /dev/sd (which is typical for USB devices)
		if strings.HasPrefix(device, "/dev/sd") {
			fmt.Printf("USB device %s is mounted at %s\n", device, mountPoint)

			// Check for valid recovery files at the mount point
			for _, validFile := range VALID_RECOVERY_FILES {
				filePath := filepath.Join(mountPoint, validFile)
				if _, err := os.Stat(filePath); err == nil {
					fmt.Printf("Found recovery file: %s\n", filePath)
					return true
				}
			}
		}
	}

	if err := scanner.Err(); err != nil {
		fmt.Printf("Error reading /proc/mounts: %v\n", err)
	}

	return false
}
