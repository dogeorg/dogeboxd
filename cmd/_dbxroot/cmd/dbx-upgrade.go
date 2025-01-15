package cmd

import (
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/spf13/cobra"
)

var dbxUpgradeCmd = &cobra.Command{
	Use:   "dbx-upgrade",
	Short: "Upgrade Dogebox to a specific release.",
	Long: `Upgrade Dogebox to a specific release.
This command requires --release flag.

Example:
  _dbxroot dbx-upgrade --package dogebox --release v0.5.0-beta`,
	Run: func(cmd *cobra.Command, args []string) {
		pkg, _ := cmd.Flags().GetString("package")
		release, _ := cmd.Flags().GetString("release")

		if pkg != "dogebox" {
			log.Printf("Invalid package to upgrade: %s", pkg)
			os.Exit(1)
		}

		if release == "" {
			log.Printf("Release tag is required")
			os.Exit(1)
		}

		upgradableReleases, err := system.GetUpgradableReleases()
		if err != nil {
			log.Printf("Failed to get upgradable releases: %v", err)
			os.Exit(1)
		}

		ok := false
		for _, upgradableRelease := range upgradableReleases {
			if upgradableRelease.Version == release {
				ok = true
				break
			}
		}

		if !ok {
			log.Printf("Release %s is not available for %s", release, pkg)
			os.Exit(1)
		}

		existingChannels := utils.RunCommand("nix-channel", "--list")

		channels := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(existingChannels), "\n") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				channels[parts[0]] = parts[1]
			}
		}

		if _, ok := channels["dogebox"]; !ok {
			log.Printf("Dogebox channel does not exist. Aborting. Please raise an issue on Github.")
			os.Exit(1)
		}

		defer func() {
			if r := recover(); r != nil {
				log.Printf("Failed to update system: %v", r)
				log.Printf("Trying to re-add old channels")

				os.Exit(1)
			}
		}()

		utils.RunCommand("nix-channel", "--remove", "dogebox")
		utils.RunCommand("nix-channel", "--add", fmt.Sprintf("https://github.com/dogeorg/dogebox-nur-packages/archive/%s.tar.gz", release), "dogebox")
		utils.RunCommand("nix-channel", "--update", "dogebox")
		utils.RunCommand("nixos-rebuild", "switch")
	},
}

func init() {
	rootCmd.AddCommand(dbxUpgradeCmd)
	dbxUpgradeCmd.Flags().StringP("package", "p", "", "Package to upgrade (required)")
	dbxUpgradeCmd.Flags().StringP("release", "r", "", "Release tag to upgrade to (required)")
	dbxUpgradeCmd.MarkFlagRequired("package")
	dbxUpgradeCmd.MarkFlagRequired("release")
}
