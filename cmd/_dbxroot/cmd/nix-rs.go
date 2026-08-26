package cmd

import (
	"fmt"
	"os"
	"os/exec"

	"github.com/Dogebox-WG/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var nixRSSetRelease string
var nixRSFlakeDir string
var nixRSGitHubToken string
var nixRSSystemdRun bool
var nixRSSystemdUnit string
var nixRSCleanupFlakeDir bool

func runCurrentSystemActivation() error {
	execCmd := exec.Command("/nix/var/nix/profiles/system/bin/switch-to-configuration", "switch")
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	return execCmd.Run()
}

func runSystemdWrappedRS() error {
	unitName := nixRSSystemdUnit
	if unitName == "" {
		unitName = "dogebox-system-update"
	}

	fmt.Fprintf(os.Stderr, "Running nixos-rebuild switch in transient unit %s; follow detailed logs with journalctl -u %s\n", unitName, unitName)

	execCmd := exec.Command("/run/current-system/sw/bin/systemd-run", buildSystemdRunRSArgs(unitName, nixRSFlakeDir, nixRSSetRelease, nixRSCleanupFlakeDir)...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	return execCmd.Run()
}

func buildSystemdRunRSArgs(unitName string, flakeDir string, setRelease string, cleanupFlakeDir bool) []string {
	systemdArgs := []string{
		"--unit", unitName,
		"--collect",
		"--wait",
		// Do not use --pipe here: dogeboxd may be stopped during activation,
		// but the transient unit must keep running to complete the switch.
		"--setenv=PATH=/run/current-system/sw/bin:/run/wrappers/bin",
		"/run/wrappers/bin/_dbxroot",
		"nix",
		"rs",
	}
	if flakeDir != "" {
		systemdArgs = append(systemdArgs, "--flake-dir", flakeDir)
	}
	if cleanupFlakeDir {
		systemdArgs = append(systemdArgs, "--cleanup-flake-dir")
	}
	if setRelease != "" {
		systemdArgs = append(systemdArgs, "--set-release", setRelease)
	}

	return systemdArgs
}

var rsCmd = &cobra.Command{
	Use:   "rs",
	Short: "Executes nixos-rebuild switch",
	Run: func(cmd *cobra.Command, args []string) {
		if nixRSSystemdRun {
			if err := runSystemdWrappedRS(); err != nil {
				fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild switch in transient unit: %v\n", err)
				os.Exit(1)
			}
			return
		}

		if err := utils.RunNixOSRebuild("switch", nixRSSetRelease, nixRSFlakeDir, nixRSGitHubToken); err != nil {
			fmt.Fprintf(os.Stderr, "Error executing nixos-rebuild switch: %v\n", err)
			os.Exit(1)
		}

		if nixRSFlakeDir != "" {
			if err := runCurrentSystemActivation(); err != nil {
				fmt.Fprintf(os.Stderr, "Error activating rebuilt system profile: %v\n", err)
				os.Exit(1)
			}
		}
		if nixRSCleanupFlakeDir && nixRSFlakeDir != "" {
			if err := os.RemoveAll(nixRSFlakeDir); err != nil {
				fmt.Fprintf(os.Stderr, "Error cleaning staged flake directory %s: %v\n", nixRSFlakeDir, err)
				os.Exit(1)
			}
		}
	},
}

func init() {
	rsCmd.Flags().StringVarP(&nixRSSetRelease, "set-release", "s", "", "rebuild with specific release (used for upgrades)")
	rsCmd.Flags().StringVar(&nixRSFlakeDir, "flake-dir", "", "rebuild from a specific flake directory")
	rsCmd.Flags().BoolVar(&nixRSSystemdRun, "systemd-run", false, "run rebuild inside a transient systemd unit")
	rsCmd.Flags().StringVar(&nixRSSystemdUnit, "systemd-unit", "", "transient systemd unit name")
	rsCmd.Flags().BoolVar(&nixRSCleanupFlakeDir, "cleanup-flake-dir", false, "remove the flake directory after a successful rebuild")
	rsCmd.Flags().StringVar(&nixRSGitHubToken, "github-token", "", "GitHub token (use for more API request allocation)")
	nixCmd.AddCommand(rsCmd)
}
