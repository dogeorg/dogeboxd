package utils

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Dogebox-WG/dogeboxd/pkg/version"
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

func buildFlakePath(baseDir string, buildType string, architecture string) string {
	if baseDir == "" {
		baseDir = "/etc/nixos"
	}

	baseDir = filepath.Clean(baseDir)
	flakeName := fmt.Sprintf("dogeboxos-%s-%s", buildType, architecture)

	return fmt.Sprintf("%s#%s", baseDir, flakeName)
}

func GetFlakePath(baseDir string) (string, error) {
	// Get system architecture
	archOutput, err := exec.Command("uname", "-m").Output()
	if err != nil {
		return "", fmt.Errorf("failed to get system architecture: %w", err)
	}
	architecture := strings.TrimSpace(string(archOutput))

	// Get build type
	buildTypeBytes, err := os.ReadFile("/opt/build-type")
	if err != nil {
		return "", fmt.Errorf("failed to read build type: %w", err)
	}
	buildType := strings.TrimSpace(string(buildTypeBytes))

	return buildFlakePath(baseDir, buildType, architecture), nil
}

func buildRebuildCommand(action string, setRelease string, flakePath string, versionInformation *version.DBXVersionInfo) (string, []string, error) {
	// Action is allowed to be "boot" or "switch". Throw an error if it's not.
	if action != "boot" && action != "switch" {
		return "", nil, fmt.Errorf("invalid action: %s", action)
	}

	commandArgs := []string{action, "--flake", flakePath, "--impure"}

	for pkg, tuple := range versionInformation.Packages {
		// Only support dogebox-wg thing for now.
		repo := fmt.Sprintf("github:dogebox-wg/%s/%s", pkg, tuple.Rev)
		// Override release (for upgrade) if setRelease is set.
		if setRelease != "" {
			repo = fmt.Sprintf("github:dogebox-wg/%s/%s", pkg, setRelease)
		}
		commandArgs = append(commandArgs, "--override-input", pkg, repo)
	}

	return "nixos-rebuild", commandArgs, nil
}

func GetRebuildCommand(action string, setRelease string, flakeDir string) (string, []string, error) {
	flakePath, err := GetFlakePath(flakeDir)
	if err != nil {
		return "", nil, fmt.Errorf("failed to get flake path: %w", err)
	}

	versionInformation := version.GetDBXRelease()
	return buildRebuildCommand(action, setRelease, flakePath, versionInformation)
}

func CopyFiles(source string, destination string) error {
	err := filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}

		destPath := filepath.Join(destination, relPath)

		if info.IsDir() {
			return os.MkdirAll(destPath, info.Mode())
		}

		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		destFile, err := os.Create(destPath)
		if err != nil {
			return err
		}
		defer destFile.Close()

		_, err = io.Copy(destFile, srcFile)
		if err != nil {
			return err
		}

		return os.Chmod(destPath, info.Mode())
	})

	return err
}
