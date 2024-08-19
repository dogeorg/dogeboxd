package system

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var VALID_RECOVERY_FILES = []string{"RECOVERY", "RECOVERY.TXT"}

// This function should do detection on whether or not we should enter our "Recovery Mode".
// This can always be overriden by a CLI flag if necessary.
func ShouldEnterRecovery() bool {
	return hasRecoveryTXT()
}

func hasRecoveryTXT() bool {
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
