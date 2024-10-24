package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
)

func IsAlphanumeric(s string) bool {
	for _, r := range s {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') {
			return false
		}
	}
	return true
}

func IsAbsolutePath(path string) bool {
	return len(path) > 0 && path[0] == '/'
}

func RunParted(device string, args ...string) {
	args = append([]string{"parted", "-s", device, "--"}, args...)
	RunCommand(args...)
}

func RunCommand(args ...string) string {
	log.Printf("----------------------------------------")
	log.Printf("Running command: %+v", args)
	cmd := exec.Command(args[0], args[1:]...)
	output := &strings.Builder{}
	cmd.Stdout = io.MultiWriter(os.Stdout, output)
	cmd.Stderr = io.MultiWriter(os.Stderr, output)
	if err := cmd.Run(); err != nil {
		log.Printf("Error running command: %v", err.Error())
		panic(err)
	}

	log.Printf("----------------------------------------")

	return output.String()
}

func GetLoopDeviceBackingFile(loopDevice string) (string, error) {
	cmd := exec.Command("losetup", "-O", "NAME,BACK-FILE", loopDevice)
	output, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to get loop device backing file: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, loopDevice) {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				return fields[1], nil
			}
		}
	}

	return "", fmt.Errorf("loop device %s not found", loopDevice)
}
