package system

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	"github.com/dogeorg/dogeboxd/pkg/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/mod/semver"
)

const RELEASE_NUR_REPO = "https://github.com/dogeorg/dogebox-nur-packages.git"

type RepositoryTag struct {
	Tag string
}

func getRepoTags(repo string) ([]RepositoryTag, error) {
	rem := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{repo},
	})

	refs, err := rem.List(&git.ListOptions{
		PeelingOption: git.AppendPeeled,
	})
	if err != nil {
		log.Printf("Failed to get repo %s tags: %v", repo, err)
		return []RepositoryTag{}, err
	}

	var tags []RepositoryTag
	for _, ref := range refs {
		if ref.Name().IsTag() && semver.IsValid(ref.Name().Short()) {
			tags = append(tags, RepositoryTag{
				Tag: ref.Name().Short(),
			})
		}
	}

	return tags, nil
}

type UpgradableRelease struct {
	Version    string
	ReleaseURL string
	Summary    string
}

func GetUpgradableReleases() ([]UpgradableRelease, error) {
	dbxRelease := version.GetDBXRelease()

	tags, err := getRepoTags(RELEASE_NUR_REPO)
	if err != nil {
		return []UpgradableRelease{}, err
	}

	unsupportedUpgradesEnabled := os.Getenv("ENABLE_UNSUPPORTED_UPGRADES") == "true"

	var upgradableTags []UpgradableRelease
	for _, tag := range tags {
		release := UpgradableRelease{
			Version:    tag.Tag,
			ReleaseURL: fmt.Sprintf("https://github.com/dogeorg/dogebox/releases/tag/%s", tag.Tag),
			Summary:    "Update for Dogeboxd / DKM / DPanel",
		}

		// Allow any version to be displayed if the user has enabled unsupported upgrades.
		// We probably want to limit this eventually, but for now it's useful for testing.
		if unsupportedUpgradesEnabled || semver.Compare(tag.Tag, dbxRelease.Release) > 0 {
			upgradableTags = append(upgradableTags, release)
		}
	}

	return upgradableTags, nil
}

func DoSystemUpdate(pkg string, updateVersion string) error {
	upgradableReleases, err := GetUpgradableReleases()
	if err != nil {
		return err
	}

	// We _only_ support the dogebox package for now.
	if pkg != "dogebox" {
		return fmt.Errorf("Invalid package to upgrade: %s", pkg)
	}

	ok := false
	for _, release := range upgradableReleases {
		if release.Version == updateVersion {
			ok = true
			break
		}
	}

	if !ok {
		return fmt.Errorf("Release %s is not available for %s", updateVersion, pkg)
	}

	cmd := exec.Command("sudo", "_dbxroot", "dbx-upgrade", "--package", pkg, "--release", updateVersion)
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run dbx-upgrade: %w", err)
	}

	// We might not even get here if dogeboxd is restarted/upgraded during this process.
	return nil
}
