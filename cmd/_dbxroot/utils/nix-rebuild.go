package utils

import (
	"os"
	"os/exec"
)

func RunNixOSRebuild(action string, setRelease string, flakeDir string) error {
	rebuildCommand, rebuildArgs, err := GetRebuildCommand(action, setRelease, flakeDir)
	if err != nil {
		return err
	}

	execCmd := exec.Command(rebuildCommand, rebuildArgs...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	if flakeDir != "" {
		execCmd.Env = append(os.Environ(), "DBX_UPGRADE_FLAKE_DIR="+flakeDir)
	}

	return execCmd.Run()
}
