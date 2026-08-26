package system

import (
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/version"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/storage/memory"
	"golang.org/x/mod/semver"
)

var RELEASE_REPOSITORY = "https://github.com/dogebox-wg/os.git"
var SUDO_COMMAND = "sudo"
var DBXROOT_WRAPPER_COMMAND = "/run/wrappers/bin/_dbxroot"

// semverSortTags sorts a slice of RepositoryTag by semver version
// direction: "desc" for descending (highest first), "asc" for ascending (lowest first)
func semverSortTags(tags []RepositoryTag, direction string) {
	sort.Slice(tags, func(i, j int) bool {
		if direction == "desc" {
			return semver.Compare(tags[i].Tag, tags[j].Tag) > 0
		}
		return semver.Compare(tags[i].Tag, tags[j].Tag) < 0
	})
}

// semverSortReleases sorts a slice of UpgradableRelease by semver version
// direction: "desc" for descending (highest first), "asc" for ascending (lowest first)
func semverSortReleases(releases []UpgradableRelease, direction string) {
	sort.Slice(releases, func(i, j int) bool {
		if direction == "desc" {
			return semver.Compare(releases[i].Version, releases[j].Version) > 0
		}
		return semver.Compare(releases[i].Version, releases[j].Version) < 0
	})
}

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

	semverSortTags(tags, "desc") // Sort descending (highest first)
	return tags, nil
}

type UpgradableRelease struct {
	Version    string
	ReleaseURL string
	Summary    string
}

type InvalidUpdatePackageError struct {
	Package string
}

func (e InvalidUpdatePackageError) Error() string {
	return fmt.Sprintf("invalid package to upgrade: %s", e.Package)
}

type UpdateVersionUnavailableError struct {
	Package string
	Version string
}

func (e UpdateVersionUnavailableError) Error() string {
	return fmt.Sprintf("release %s is not available for %s", e.Version, e.Package)
}

// RepoTagsFetcher interface for mocking getRepoTags
type RepoTagsFetcher interface {
	GetRepoTags(repo string) ([]RepositoryTag, error)
}

// DefaultRepoTagsFetcher implements RepoTagsFetcher using the actual git implementation
type DefaultRepoTagsFetcher struct{}

func (d *DefaultRepoTagsFetcher) GetRepoTags(repo string) ([]RepositoryTag, error) {
	return getRepoTags(repo)
}

// Global variable to allow dependency injection for testing
var repoTagsFetcher RepoTagsFetcher = &DefaultRepoTagsFetcher{}

func GetUpgradableReleases(includePreReleases bool) ([]UpgradableRelease, error) {
	return GetUpgradableReleasesWithFetcher(includePreReleases, repoTagsFetcher)
}

func GetUpgradableReleasesWithFetcher(includePreReleases bool, fetcher RepoTagsFetcher) ([]UpgradableRelease, error) {
	dbxRelease := version.GetDBXRelease()
	return GetUpgradableReleasesForVersionWithFetcher(dbxRelease.Release, includePreReleases, fetcher)
}

func GetUpgradableReleasesForVersionWithFetcher(currentRelease string, includePreReleases bool, fetcher RepoTagsFetcher) ([]UpgradableRelease, error) {
	tags, err := fetcher.GetRepoTags(RELEASE_REPOSITORY)
	if err != nil {
		return []UpgradableRelease{}, err
	}

	var upgradableTags []UpgradableRelease
	for _, tag := range tags {
		release := UpgradableRelease{
			Version:    tag.Tag,
			ReleaseURL: fmt.Sprintf("https://github.com/dogebox-wg/os/releases/tag/%s", tag.Tag),
			Summary:    "Update for Dogeboxd / DKM / DPanel",
		}

		if semver.Compare(tag.Tag, currentRelease) > 0 {
			// If not including pre-releases, filter out pre-release versions
			if !includePreReleases && semver.Prerelease(tag.Tag) != "" {
				continue
			}
			upgradableTags = append(upgradableTags, release)
		}
	}

	semverSortReleases(upgradableTags, "desc") // Sort descending (highest first)

	return upgradableTags, nil
}

func cloneReleaseRepository(destination, version string) error {
	_, err := git.PlainClone(destination, false, &git.CloneOptions{
		URL:           RELEASE_REPOSITORY,
		ReferenceName: plumbing.NewTagReferenceName(version),
		SingleBranch:  true,
		Depth:         1,
	})
	if err != nil {
		return fmt.Errorf("failed to clone OS release %s: %w", version, err)
	}

	return nil
}

func getRepositoryHeadHash(repoDir string) (string, error) {
	repo, err := git.PlainOpen(repoDir)
	if err != nil {
		return "", fmt.Errorf("failed to open cloned repository %s: %w", repoDir, err)
	}

	head, err := repo.Head()
	if err != nil {
		return "", fmt.Errorf("failed to read cloned repository HEAD: %w", err)
	}

	return head.Hash().String(), nil
}

func buildStagedReleaseDirPath(tmpDir string, updateVersion string, commitHash string) string {
	return filepath.Join(tmpDir, fmt.Sprintf("os-upgrade-%s-%s", updateVersion, commitHash))
}

func sanitizeSystemdUnitComponent(value string) string {
	sanitizer := strings.NewReplacer(".", "-", "/", "-", " ", "-", ":", "-", "@", "-", "+", "-", "=", "-")
	return sanitizer.Replace(value)
}

func shortCommitHash(commitHash string) string {
	if len(commitHash) <= 8 {
		return commitHash
	}

	return commitHash[:8]
}

func buildSystemUpdateUnitName(updateVersion string, commitHash string) string {
	unitName := fmt.Sprintf("dogebox-system-update-%s", sanitizeSystemdUnitComponent(updateVersion))
	if commitHash != "" {
		unitName = fmt.Sprintf("%s-%s", unitName, sanitizeSystemdUnitComponent(shortCommitHash(commitHash)))
	}
	return unitName
}

func buildSystemUpdateCommandArgs(stagedFlakeDir string, updateVersion string, unitName string) []string {
	args := []string{
		DBXROOT_WRAPPER_COMMAND,
		"nix",
		"rs",
		"--systemd-run",
		"--systemd-unit",
		unitName,
		"--flake-dir",
		stagedFlakeDir,
		// Activation reads the staged flake after dogeboxd may have been
		// stopped, so cleanup must happen inside _dbxroot's transient unit.
		"--cleanup-flake-dir",
		"--set-release",
		updateVersion,
	}
	if token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN")); token != "" {
		args = append(args, "--github-token", token)
	}
	return args
}

func redactGitHubTokenInArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)
	for i := 0; i < len(out)-1; i++ {
		if out[i] == "--github-token" {
			out[i+1] = "[REDACTED]"
		}
	}
	return out
}

func stageReleaseFlake(tmpDir, updateVersion string, logger dogeboxd.SubLogger) (string, string, error) {
	return stageReleaseFlakeWithClone(tmpDir, updateVersion, logger, cloneReleaseRepository)
}

func stageReleaseFlakeWithClone(tmpDir, updateVersion string, logger dogeboxd.SubLogger, cloneFunc func(string, string) error) (string, string, error) {
	if tmpDir == "" {
		tmpDir = os.TempDir()
	}

	cloneDir, err := os.MkdirTemp(tmpDir, "os-upgrade-*")
	if err != nil {
		return "", "", fmt.Errorf("failed to create temp dir for OS release %s: %w", updateVersion, err)
	}

	if logger != nil {
		logger.Logf("Cloning OS release %s into %s", updateVersion, cloneDir)
	}

	if err := cloneFunc(cloneDir, updateVersion); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", "", err
	}

	flakePath := filepath.Join(cloneDir, "flake.nix")
	if _, err := os.Stat(flakePath); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", "", fmt.Errorf("staged OS release %s is missing flake.nix: %w", updateVersion, err)
	}

	commitHash, err := getRepositoryHeadHash(cloneDir)
	if err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", "", err
	}

	if err := os.RemoveAll(filepath.Join(cloneDir, ".git")); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", "", fmt.Errorf("failed to strip Git metadata from staged OS release: %w", err)
	}

	finalDir := buildStagedReleaseDirPath(tmpDir, updateVersion, commitHash)
	if err := os.RemoveAll(finalDir); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", "", fmt.Errorf("failed to clear existing staged OS release dir %s: %w", finalDir, err)
	}

	if err := os.Rename(cloneDir, finalDir); err != nil {
		_ = os.RemoveAll(cloneDir)
		return "", "", fmt.Errorf("failed to move staged OS release into %s: %w", finalDir, err)
	}

	return finalDir, commitHash, nil
}

func doSystemUpdate(pkg string, updateVersion string, tmpDir string, logger dogeboxd.SubLogger) error {
	return doSystemUpdateWithDependencies(pkg, updateVersion, tmpDir, logger, cloneReleaseRepository, exec.Command)
}

func doSystemUpdateWithDependencies(
	pkg string,
	updateVersion string,
	tmpDir string,
	logger dogeboxd.SubLogger,
	cloneFunc func(string, string) error,
	execCommand func(string, ...string) *exec.Cmd,
) error {
	upgradableReleases, err := GetUpgradableReleases(true)
	if err != nil {
		return err
	}

	// We _only_ support the "os" package for now.
	if pkg != "os" {
		return InvalidUpdatePackageError{Package: pkg}
	}

	ok := false
	for _, release := range upgradableReleases {
		if release.Version == updateVersion {
			ok = true
			break
		}
	}

	if !ok {
		return UpdateVersionUnavailableError{Package: pkg, Version: updateVersion}
	}

	stagedFlakeDir, commitHash, err := stageReleaseFlakeWithClone(tmpDir, updateVersion, logger, cloneFunc)
	if err != nil {
		return err
	}

	cmd := execCommand(SUDO_COMMAND, buildSystemUpdateCommandArgs(stagedFlakeDir, updateVersion, buildSystemUpdateUnitName(updateVersion, commitHash))...)
	if logger != nil {
		logger.Logf("Running command: %s %s", cmd.Path, strings.Join(redactGitHubTokenInArgs(cmd.Args[1:]), " "))
		cmd.Stdout = io.MultiWriter(os.Stdout, dogeboxd.NewLineWriter(func(s string) {
			logger.Log(s)
		}))
		cmd.Stderr = io.MultiWriter(os.Stderr, dogeboxd.NewLineWriter(func(s string) {
			logger.Log(s)
		}))
	} else {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to run dbx-upgrade: %w", err)
	}

	// We probably won't even get here if dogeboxd is restarted/upgraded during this process.
	return err
}

func (t SystemUpdater) DoSystemUpdate(pkg string, updateVersion string, logger dogeboxd.SubLogger) error {
	if err := MigrateLegacyCustomNix(t.config); err != nil {
		return err
	}

	return doSystemUpdate(pkg, updateVersion, t.config.TmpDir, logger)
}

func DoSystemUpdate(pkg string, updateVersion string, logger dogeboxd.SubLogger) error {
	return doSystemUpdate(pkg, updateVersion, "", logger)
}
