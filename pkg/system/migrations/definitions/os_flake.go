package definitions

import (
	"fmt"
	"log"
	"os"
	"regexp"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations/core"
	"github.com/Dogebox-WG/dogeboxd/pkg/version"
)

var installedOSFlakePath = "/etc/nixos/flake.nix"

const osFlakeIncludePreReleasesConfigKey = "includePreReleases"

var osFlakeMigrationMetadata = struct {
	Name    string
	Version string
}{
	Name:    "pre_0.9_os_flake",
	Version: "v0.9.0-rc.1",
}

var OSFlakeMigration = core.Migration{
	Name:         osFlakeMigrationMetadata.Name,
	DisplayName:  "OS flake migrator",
	Version:      osFlakeMigrationMetadata.Version,
	Requirements: osFlakeMigrationRequirements,
	Run:          runOSFlakeMigration,
}

// This migration handles upgrading to the v0.9.0 OS flake. Those
// systems may boot into a new dogeboxd package while still running an older
// /etc/nixos flake, because the old updater did not pass a staged flake
// directory into the rebuild step.
//
// The migration process:
//  1. read the installed OS flake version from /etc/nixos/flake.nix,
//  2. require an installed OS flake version matching <=v0.9.0-rc.1,
//  3. select the latest eligible OS release,
//  4. compare that target against the installed OS flake version,
//  5. queue the normal SystemUpdate path when the target OS flake still needs to land.

var osFlakeVersionPattern = regexp.MustCompile(`(?m)^\s*dbxRelease\s*=\s*"(v[^"]+)"`)

type osFlakeMigrationDecision struct {
	installedFlakeVersion string
	currentDBXRelease     string
	targetVersion         string
	includePreReleases    bool
	complete              bool
	reason                string
}

func getInstalledOSFlakeVersion(readFile func(string) ([]byte, error), flakePath string) (string, error) {
	flakeContents, err := readFile(flakePath)
	if err != nil {
		return "", err
	}

	match := osFlakeVersionPattern.FindStringSubmatch(string(flakeContents))
	if len(match) != 2 {
		return "", fmt.Errorf("installed os flake version not found in %s", flakePath)
	}
	if err := core.ValidateSemverVersion(match[1], "installed os flake"); err != nil {
		return "", fmt.Errorf("installed os flake version %q is not valid semver", match[1])
	}
	return match[1], nil
}

func semverIsAtOrAbove(version string, threshold string) (bool, error) {
	if version == "" {
		return false, nil
	}

	return core.VersionConstraintCheck(version, ">="+threshold)
}

func isOSFlakeMigrationComplete(installedFlakeVersion string, currentDBXRelease string) (bool, error) {
	for _, candidate := range []string{installedFlakeVersion, currentDBXRelease} {
		ok, err := semverIsAtOrAbove(candidate, osFlakeMigrationMetadata.Version)
		if err != nil {
			return false, err
		}
		if !ok {
			return false, nil
		}
	}

	return true, nil
}

func osFlakeMigrationRequirements(ctx core.Context, record core.MigrationRecord) (bool, string, error) {
	if ctx.Config.Recovery {
		return false, "running in recovery mode", nil
	}

	decision, err := determineOSFlakeMigrationDecision(ctx, record, true)
	if err != nil {
		return false, "", err
	}
	if decision.complete {
		if err := core.MarkCompleted(ctx.Config, osFlakeMigrationMetadata.Name); err != nil {
			return false, "", err
		}
		log.Printf(
			"OS flake migrator marking migration complete after verifying target state: installed OS flake=%s, current DBX release=%s",
			decision.installedFlakeVersion,
			decision.currentDBXRelease,
		)
		return false, "verified target state already satisfies the migration", nil
	}
	if decision.reason != "" {
		return false, decision.reason, nil
	}

	return true, "", nil
}

func runOSFlakeMigration(ctx core.Context, record core.MigrationRecord) (string, bool, error) {
	decision, err := determineOSFlakeMigrationDecision(ctx, record, false)
	if err != nil {
		return "", false, err
	}
	if decision.complete {
		return "", false, nil
	}

	activeJobs, err := ctx.ActiveJobsOrDefault()()
	if err != nil {
		return "", false, err
	}
	for _, job := range activeJobs {
		if job.Action == (dogeboxd.SystemUpdate{}).ActionName() {
			log.Printf("Skipping OS flake migrator queue because active job %s (%s) is already %s", job.ID, job.Action, job.Status)
			return "", false, nil
		}
	}

	if decision.targetVersion == "" {
		return "", false, nil
	}

	jobID := ctx.Enqueue(dogeboxd.SystemUpdate{
		Package: "os",
		Version: decision.targetVersion,
	})

	return jobID, true, nil
}

func determineOSFlakeMigrationDecision(ctx core.Context, record core.MigrationRecord, logDiscovery bool) (osFlakeMigrationDecision, error) {
	installedFlakeVersion, err := getInstalledOSFlakeVersion(ctx.ReadFileOrDefault(), installedOSFlakePath)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}

	currentDBXRelease := version.GetDBXRelease().Release

	includePreReleases := record.BoolConfig(osFlakeIncludePreReleasesConfigKey)
	complete, err := isOSFlakeMigrationComplete(installedFlakeVersion, currentDBXRelease)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}

	decision := osFlakeMigrationDecision{
		installedFlakeVersion: installedFlakeVersion,
		currentDBXRelease:     currentDBXRelease,
		includePreReleases:    includePreReleases,
		complete:              complete,
	}

	if complete {
		return decision, nil
	}

	releases, err := system.GetUpgradableReleasesForVersionWithFetcher(currentDBXRelease, includePreReleases, ctx.RepoTagsFetcherOrDefault())
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	if logDiscovery {
		latestEligibleRelease := "none"
		if len(releases) > 0 {
			latestEligibleRelease = releases[0].Version
		}
		log.Printf(
			"OS flake migrator release discovery: installed OS flake=%s, current DBX release=%s, includePreReleases=%t, latest eligible OS release=%s",
			installedFlakeVersion,
			currentDBXRelease,
			includePreReleases,
			latestEligibleRelease,
		)
	}

	matchesInstalledVersionConstraint, err := core.VersionConstraintCheck(installedFlakeVersion, "<="+osFlakeMigrationMetadata.Version)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	if !matchesInstalledVersionConstraint {
		decision.reason = fmt.Sprintf("installed OS flake version %s is newer than %s", installedFlakeVersion, osFlakeMigrationMetadata.Version)
		return decision, nil
	}

	currentReleaseInMigrationRange, err := semverIsAtOrAbove(currentDBXRelease, osFlakeMigrationMetadata.Version)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	if currentReleaseInMigrationRange {
		// A partial activation can update DBX while leaving /etc/nixos on the
		// old flake. Re-run the current release so activation can copy the
		// staged flake into place and let the next startup mark this complete.
		decision.targetVersion = currentDBXRelease
		return decision, nil
	}

	if len(releases) == 0 {
		preReleasePolicy := "pre-releases excluded"
		if includePreReleases {
			preReleasePolicy = "pre-releases included"
		}
		decision.reason = fmt.Sprintf("no eligible OS releases are available newer than current DBX release %s (%s)", currentDBXRelease, preReleasePolicy)
		return decision, nil
	}

	targetVersion := releases[0].Version
	targetIsNewer, err := core.VersionConstraintCheck(installedFlakeVersion, "<"+targetVersion)
	if err != nil {
		return osFlakeMigrationDecision{}, err
	}
	if !targetIsNewer {
		decision.targetVersion = targetVersion
		decision.reason = fmt.Sprintf("installed OS flake version %s is not older than inferred target %s", installedFlakeVersion, targetVersion)
		return decision, nil
	}

	decision.targetVersion = targetVersion
	return decision, nil
}

func QueueOSFlakeMigratorIfNeeded(
	config dogeboxd.ServerConfig,
	enqueue func(dogeboxd.Action) string,
	activeJobs func() ([]dogeboxd.JobRecord, error),
) (string, bool, error) {
	return core.RunMigrations(core.Context{
		Config:          config,
		Enqueue:         enqueue,
		ActiveJobs:      activeJobs,
		ReadFile:        os.ReadFile,
		RepoTagsFetcher: &system.DefaultRepoTagsFetcher{},
	}, []core.Migration{OSFlakeMigration})
}
