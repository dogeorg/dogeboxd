package utils

import (
	"os"
	"os/exec"
	"strings"
)

func RunNixOSRebuild(action string, setRelease string, flakeDir string, githubToken string) error {
	rebuildCommand, rebuildArgs, err := GetRebuildCommand(action, setRelease, flakeDir)
	if err != nil {
		return err
	}

	execCmd := exec.Command(rebuildCommand, rebuildArgs...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr

	env := os.Environ()
	var extraEnv []string

	if githubToken != "" {
		nixConfig := strings.TrimSpace(os.Getenv("NIX_CONFIG"))
		accessTokenLine := "access-tokens = github.com=" + githubToken
		if nixConfig == "" {
			nixConfig = accessTokenLine
		} else if !strings.Contains(nixConfig, "github.com=") {
			nixConfig = nixConfig + "\n" + accessTokenLine
		}
		extraEnv = append(extraEnv, "NIX_CONFIG="+nixConfig)
	}

	if flakeDir != "" {
		extraEnv = append(extraEnv, "DBX_UPGRADE_FLAKE_DIR="+flakeDir)
	}

	if len(extraEnv) > 0 {
		execCmd.Env = append(env, extraEnv...)
	}

	return execCmd.Run()
}
