package definitions

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/migrations/core"
)

type mockRepoTagsFetcher struct {
	tags []system.RepositoryTag
	err  error
}

func (m mockRepoTagsFetcher) GetRepoTags(string) ([]system.RepositoryTag, error) {
	return m.tags, m.err
}

func testOSFlakeReadFiles(files map[string]string) func(string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		content, ok := files[path]
		if !ok {
			return nil, os.ErrNotExist
		}
		return []byte(content), nil
	}
}

func setupTestDBXRelease(t *testing.T, currentRelease string) {
	t.Helper()

	versionDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(versionDir, "dbx"), []byte(currentRelease+"\n"), 0644); err != nil {
		t.Fatalf("failed to write test dbx release: %v", err)
	}
	t.Setenv("VERSION_PATH_OVERRIDE", versionDir)
}

func testFlakeVersionFile(version string) string {
	return fmt.Sprintf(`{
  dbxRelease = %q;
}`, version)
}

func testOSFlakeFiles(installedVersion string) map[string]string {
	return map[string]string{
		installedOSFlakePath: testFlakeVersionFile(installedVersion),
	}
}

func TestOSFlakeMigrationRequirementsSkipWhenInstalledVersionIsAfterConstraint(t *testing.T) {
	setupTestDBXRelease(t, "v0.9.0")

	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile: testOSFlakeReadFiles(testOSFlakeFiles("v0.9.0")),
		RepoTagsFetcher: mockRepoTagsFetcher{
			tags: []system.RepositoryTag{
				{Tag: "v1.2.0"},
			},
		},
	}

	applies, _, err := OSFlakeMigration.Requirements(ctx, core.MigrationRecord{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if applies {
		t.Fatal("expected installed version after constraint to skip migration")
	}

	state, err := core.LoadState(ctx.Config)
	if err != nil {
		t.Fatalf("expected load to succeed, got %v", err)
	}
	if !state[osFlakeMigrationMetadata.Name].DoNotRun {
		t.Fatalf("expected migration to mark doNotRun after verified completion, got %+v", state[osFlakeMigrationMetadata.Name])
	}
	if !state[osFlakeMigrationMetadata.Name].RanSuccessfully {
		t.Fatalf("expected migration to mark ranSuccessfully after verified completion, got %+v", state[osFlakeMigrationMetadata.Name])
	}
}

func TestOSFlakeMigrationRequirementsSkipWhenNoStableReleaseAvailable(t *testing.T) {
	setupTestDBXRelease(t, "v0.8.1")

	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile: testOSFlakeReadFiles(testOSFlakeFiles("v0.9.0-rc.1")),
		RepoTagsFetcher: mockRepoTagsFetcher{
			tags: []system.RepositoryTag{
				{Tag: "v1.2.0-rc.1"},
			},
		},
	}

	applies, reason, err := OSFlakeMigration.Requirements(ctx, core.MigrationRecord{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if applies {
		t.Fatal("expected no stable release to skip migration")
	}
	if reason == "" {
		t.Fatal("expected skip reason")
	}
	if reason != "no eligible OS releases are available newer than current DBX release v0.8.1 (pre-releases excluded)" {
		t.Fatalf("expected skip reason to reference current DBX release, got %q", reason)
	}
}

func TestRunOSFlakeMigrationQueuesLatestPrereleaseWhenEnabled(t *testing.T) {
	setupTestDBXRelease(t, "v0.8.1")

	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile: testOSFlakeReadFiles(testOSFlakeFiles("v0.8.1")),
		RepoTagsFetcher: mockRepoTagsFetcher{
			tags: []system.RepositoryTag{
				{Tag: "v1.3.0-rc.2"},
				{Tag: "v1.2.0"},
				{Tag: "v1.1.0"},
			},
		},
	}

	var queuedActions []dogeboxd.Action
	ctx.Enqueue = func(action dogeboxd.Action) string {
		queuedActions = append(queuedActions, action)
		return "job-prerelease"
	}

	jobID, queued, err := OSFlakeMigration.Run(ctx, core.MigrationRecord{
		Config: map[string]any{
			osFlakeIncludePreReleasesConfigKey: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !queued {
		t.Fatal("expected migration to queue a prerelease update")
	}
	if jobID != "job-prerelease" {
		t.Fatalf("expected job-prerelease, got %s", jobID)
	}
	if len(queuedActions) != 1 {
		t.Fatalf("expected exactly one queued action, got %d", len(queuedActions))
	}

	update, ok := queuedActions[0].(dogeboxd.SystemUpdate)
	if !ok {
		t.Fatalf("expected SystemUpdate action, got %T", queuedActions[0])
	}
	if update.Package != "os" || update.Version != "v1.3.0-rc.2" {
		t.Fatalf("unexpected queued update: %+v", update)
	}
}

func TestRunOSFlakeMigrationQueuesCurrentReleaseWhenDBXUpdatedButFlakeIsStale(t *testing.T) {
	setupTestDBXRelease(t, "v0.9.0-rc.8")

	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile:        testOSFlakeReadFiles(testOSFlakeFiles("v0.8.2")),
		RepoTagsFetcher: mockRepoTagsFetcher{},
	}

	var queuedActions []dogeboxd.Action
	ctx.Enqueue = func(action dogeboxd.Action) string {
		queuedActions = append(queuedActions, action)
		return "job-current-release"
	}

	jobID, queued, err := OSFlakeMigration.Run(ctx, core.MigrationRecord{
		Config: map[string]any{
			osFlakeIncludePreReleasesConfigKey: true,
		},
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !queued {
		t.Fatal("expected migration to queue a current-release repair update")
	}
	if jobID != "job-current-release" {
		t.Fatalf("expected job-current-release, got %s", jobID)
	}
	if len(queuedActions) != 1 {
		t.Fatalf("expected exactly one queued action, got %d", len(queuedActions))
	}

	update, ok := queuedActions[0].(dogeboxd.SystemUpdate)
	if !ok {
		t.Fatalf("expected SystemUpdate action, got %T", queuedActions[0])
	}
	if update.Package != "os" || update.Version != "v0.9.0-rc.8" {
		t.Fatalf("unexpected queued update: %+v", update)
	}
}

func TestRunOSFlakeMigrationSkipsWhenSystemJobAlreadyActive(t *testing.T) {
	setupTestDBXRelease(t, "v0.8.1")

	enqueued := false
	ctx := core.Context{
		Config: dogeboxd.ServerConfig{
			DataDir: t.TempDir(),
			TmpDir:  t.TempDir(),
		},
		ReadFile: testOSFlakeReadFiles(testOSFlakeFiles("v0.8.1")),
		RepoTagsFetcher: mockRepoTagsFetcher{
			tags: []system.RepositoryTag{
				{Tag: "v0.9.0-rc.8"},
			},
		},
		ActiveJobs: func() ([]dogeboxd.JobRecord, error) {
			return []dogeboxd.JobRecord{
				{
					ID:     "job-1",
					Action: dogeboxd.SystemUpdate{}.ActionName(),
					Status: dogeboxd.JobStatusInProgress,
				},
			}, nil
		},
		Enqueue: func(action dogeboxd.Action) string {
			enqueued = true
			return "job-should-not-queue"
		},
	}

	jobID, queued, err := OSFlakeMigration.Run(ctx, core.MigrationRecord{})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if queued || jobID != "" {
		t.Fatalf("expected migration to skip duplicate active work, got queued=%v jobID=%q", queued, jobID)
	}
	if enqueued {
		t.Fatal("expected migration not to enqueue while system job is active")
	}
}
