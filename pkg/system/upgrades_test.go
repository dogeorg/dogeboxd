package system

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"golang.org/x/mod/semver"
)

// MockRepoTagsFetcher implements RepoTagsFetcher for testing
type MockRepoTagsFetcher struct {
	tags []RepositoryTag
	err  error
}

func (m *MockRepoTagsFetcher) GetRepoTags(repo string) ([]RepositoryTag, error) {
	return m.tags, m.err
}

func TestRedactGitHubTokenInArgs(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "no token flag",
			in:   []string{"sudo", "_dbxroot", "nix", "rs", "--flake-dir", "/tmp/x"},
			want: []string{"sudo", "_dbxroot", "nix", "rs", "--flake-dir", "/tmp/x"},
		},
		{
			name: "github token redacted",
			in:   []string{"sudo", "_dbxroot", "nix", "rs", "--github-token", "ghp_secret"},
			want: []string{"sudo", "_dbxroot", "nix", "rs", "--github-token", "[REDACTED]"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := redactGitHubTokenInArgs(tc.in)
			if len(got) != len(tc.want) {
				t.Fatalf("len got %d want %d", len(got), len(tc.want))
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("index %d: got %q want %q", i, got[i], tc.want[i])
				}
			}
			// Original slice must be unchanged (copy semantics).
			if len(tc.in) >= 2 && tc.in[len(tc.in)-1] == "ghp_secret" {
				if tc.in[len(tc.in)-1] != "ghp_secret" {
					t.Error("input slice was mutated")
				}
			}
		})
	}
}

// TODO : this only tests the semver module
func TestSemverValidation(t *testing.T) {
	testCases := []struct {
		version string
		valid   bool
	}{
		{"v1.0.0", true},
		{"v1.2.3", true},
		{"v10.20.30", true},
		{"1.0.0", false}, // semver.IsValid requires 'v' prefix in this context
		{"invalid", false},
		{"v1.2", true}, // we're allowing 'vMAJOR.MINOR' (semver equates this to 'vMAJOR.MINOR.0')
		{"v1.2.3.4", false},
	}

	for _, tc := range testCases {
		t.Run(tc.version, func(t *testing.T) {
			isValid := semver.IsValid(tc.version)
			if isValid != tc.valid {
				t.Errorf("For version %s, expected valid=%v, got valid=%v", tc.version, tc.valid, isValid)
			}
		})
	}
}

func TestGenerateSequentialVersions(t *testing.T) {
	tags := GenerateSequentialVersions(1, 0, 3)
	expectedTags := []string{"v1.0.0", "v1.1.0", "v1.2.0"}

	if len(tags) != len(expectedTags) {
		t.Errorf("Expected %d tags, got %d", len(expectedTags), len(tags))
	}

	for i, expectedTag := range expectedTags {
		if tags[i].Tag != expectedTag {
			t.Errorf("Expected tag[%d] to be %s, got %s", i, expectedTag, tags[i].Tag)
		}
	}
}

// TODO : this just tests MockRepoTagsFetcher, is this or SuccessMock/Errormock from helpers even needed?
func TestMockFetcherCreationHelpers(t *testing.T) {
	// Test CreateSuccessMock
	tags := []RepositoryTag{{Tag: "v1.0.0"}, {Tag: "v1.1.0"}}
	successMock := CreateSuccessMock(tags)

	result, err := successMock.GetRepoTags("test-repo")
	if err != nil {
		t.Errorf("Expected no error from success mock, got: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 tags, got %d", len(result))
	}

	// Test CreateErrorMock
	testErr := fmt.Errorf("test error")
	errorMock := CreateErrorMock(testErr)

	result, err = errorMock.GetRepoTags("test-repo")
	if err == nil {
		t.Error("Expected error from error mock, got nil")
	}

	if err.Error() != "test error" {
		t.Errorf("Expected 'test error', got '%s'", err.Error())
	}

	if len(result) != 0 {
		t.Errorf("Expected 0 tags on error, got %d", len(result))
	}
}

// TODO : This is a trivial function, so maybe this is doing too much?
func TestMockRepoTagsFetcher(t *testing.T) {
	// Test successful fetch
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},
		{Tag: "v1.1.0"},
		{Tag: "v1.2.0"},
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	result, err := mockFetcher.GetRepoTags("test-repo")
	if err != nil {
		t.Errorf("Expected no error, got: %v", err)
	}
	if len(result) != 3 {
		t.Errorf("Expected 3 tags, got %d", len(result))
	}
	if result[0].Tag != "v1.0.0" {
		t.Errorf("Expected first tag to be 'v1.0.0', got '%s'", result[0].Tag)
	}

	// Test error case
	errorFetcher := &MockRepoTagsFetcher{
		tags: nil,
		err:  fmt.Errorf("test error"),
	}
	result, err = errorFetcher.GetRepoTags("test-repo")
	if err == nil {
		t.Error("Expected error, got nil")
	}
	if len(result) != 0 {
		t.Errorf("Expected 0 tags on error, got %d", len(result))
	}
}

// GetUpgradableReleasesWithFetcher with getRepoTags mocked,
// normal case with upgrade versions available
func TestGetUpgradableReleases_UpgradesAvailable(t *testing.T) {
	// Create a mock fetcher with test data
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},
		{Tag: "v1.1.0"},
		{Tag: "v1.2.0"},
		{Tag: "v2.0.0"},
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Create temporary version file to simulate current version
	tempDir := setupMockVersioning(t, "v1.1.0")
	defer os.RemoveAll(tempDir)

	releases, err := GetUpgradableReleasesWithFetcher(false, mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should only return versions newer than v1.1.0 (v1.2.0 and v2.0.0)
	expectedCount := 2
	if len(releases) != expectedCount {
		t.Errorf("Expected %d upgradable releases, got %d", expectedCount, len(releases))
	}

	// Check first upgrade (v2.0.0 - highest version first)
	if releases[0].Version != "v2.0.0" {
		t.Errorf("Expected first upgrade to be v2.0.0, got %s", releases[0].Version)
	}

	// Check second upgrade (v1.2.0)
	if releases[1].Version != "v1.2.0" {
		t.Errorf("Expected second upgrade to be v1.2.0, got %s", releases[1].Version)
	}

	// Verify release URLs are correctly formatted
	expectedURL1 := "https://github.com/dogebox-wg/os/releases/tag/v2.0.0"
	if releases[0].ReleaseURL != expectedURL1 {
		t.Errorf("Expected release URL to be %s, got %s", expectedURL1, releases[0].ReleaseURL)
	}

	// Verify all releases have the expected summary
	for _, release := range releases {
		expectedSummary := "Update for Dogeboxd / DKM / DPanel"
		if release.Summary != expectedSummary {
			t.Errorf("Expected summary to be '%s', got '%s'", expectedSummary, release.Summary)
		}
	}
}

// GetUpgradableReleasesWithFetcher with getRepoTags mocked,
// normal case with no upgrade versions available
func TestGetUpgradableReleases_NoUpgrades(t *testing.T) {
	// Test when current version is newer than all available versions
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},
		{Tag: "v1.1.0"},
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version to be newer than available tags
	tempDir := setupMockVersioning(t, "v2.0.0")
	defer os.RemoveAll(tempDir)

	releases, err := GetUpgradableReleasesWithFetcher(false, mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(releases) != 0 {
		t.Errorf("Expected no upgradable releases, got %d", len(releases))
	}
}

// GetUpgradableReleasesWithFetcher with getRepoTags mocked,
// no tags in repository.
func TestGetUpgradableReleases_EmptyTags(t *testing.T) {
	// Test with empty tags
	mockFetcher := &MockRepoTagsFetcher{tags: []RepositoryTag{}, err: nil}

	tempDir := setupMockVersioning(t, "v1.0.0")
	defer os.RemoveAll(tempDir)

	releases, err := GetUpgradableReleasesWithFetcher(false, mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if len(releases) != 0 {
		t.Errorf("Expected no releases with empty tags, got %d", len(releases))
	}
}

// TODO : This appears to test an error condition by feeding an error /in/ to GetUpgradableReleasesWithFetcher
// and checks that the same error propogates?
func TestGetUpgradableReleases_WithErrorFromMock(t *testing.T) {
	// Test with error from mock
	mockFetcher := &MockRepoTagsFetcher{
		tags: nil,
		err:  fmt.Errorf("mock error: failed to fetch tags"),
	}

	releases, err := GetUpgradableReleasesWithFetcher(false, mockFetcher)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	if len(releases) != 0 {
		t.Errorf("Expected empty releases slice on error, got %d releases", len(releases))
	}

	expectedErrorMsg := "mock error: failed to fetch tags"
	if err.Error() != expectedErrorMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedErrorMsg, err.Error())
	}
}

// TestTableDrivenUpgradeScenarios tests various upgrade scenarios using table-driven approach
func TestIntegrationSuite(t *testing.T) {
	testCases := []TableDrivenTest{
		{
			Name:             "Single upgrade available",
			CurrentVersion:   "v1.0.0",
			AvailableTags:    []RepositoryTag{{Tag: "v1.1.0"}},
			ExpectedCount:    1,
			ExpectedVersions: []string{"v1.1.0"},
		},
		{
			Name:           "Multiple pre-release and release versions",
			CurrentVersion: "v1.0.0",
			AvailableTags: []RepositoryTag{
				// TODO : There's nothing to discard tags with invalid semver, do we want this?
				//{Tag: "v1.1.0-alpha"}, // Should be ignored (invalid semver without rc/beta/alpha handling)
				{Tag: "v1.1.0"},
				{Tag: "v1.2.0"},
			},
			ExpectedCount:    2,
			ExpectedVersions: []string{"v1.2.0", "v1.1.0"},
		},
		{
			Name:           "Empty repository",
			CurrentVersion: "v1.0.0",
			AvailableTags:  []RepositoryTag{},
			ExpectedCount:  0,
		},
		{
			Name:           "Multiple upgrades available",
			CurrentVersion: "v1.0.0",
			AvailableTags: []RepositoryTag{
				{Tag: "v0.9.0"},
				{Tag: "v1.0.0"},
				{Tag: "v1.1.0"},
				{Tag: "v1.2.0"},
				{Tag: "v2.0.0"},
			},
			ExpectedCount:    3,
			ExpectedVersions: []string{"v2.0.0", "v1.2.0", "v1.1.0"},
		},
		{
			Name:           "No upgrades available - current is latest",
			CurrentVersion: "v2.0.0",
			AvailableTags: []RepositoryTag{
				{Tag: "v1.0.0"},
				{Tag: "v1.1.0"},
				{Tag: "v2.0.0"},
			},
			ExpectedCount: 0,
		},
		{
			Name:           "No upgrades available - current is newer",
			CurrentVersion: "v3.0.0",
			AvailableTags: []RepositoryTag{
				{Tag: "v1.0.0"},
				{Tag: "v2.0.0"},
			},
			ExpectedCount: 0,
		},
		{
			Name:           "Network error",
			CurrentVersion: "v1.0.0",
			MockError:      ErrNetworkTimeout,
			ExpectedError:  "network timeout",
		},
		{
			Name:           "Repository not found",
			CurrentVersion: "v1.0.0",
			MockError:      ErrRepoNotFound,
			ExpectedError:  "repository not found",
		},
	}

	RunTableDrivenTests(t, testCases)
}

// TODO : Unable to test a working case of DoSystemUpdate

// specifying invalid package name
func TestDoSystemUpdate_InvalidPackage(t *testing.T) {
	// Mock successful tag retrieval
	mockTags := []RepositoryTag{
		{Tag: "v1.2.0"},
	}

	// Store original fetcher and restore after test
	originalFetcher := repoTagsFetcher
	defer func() {
		repoTagsFetcher = originalFetcher
	}()

	repoTagsFetcher = &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version
	tempDir := setupMockVersioning(t, "v1.1.0")
	defer os.RemoveAll(tempDir)

	err := DoSystemUpdate("invalid-package", "v1.2.0", nil)
	if err == nil {
		t.Fatal("expected error for invalid package, got nil")
	}

	expectedError := "invalid package to upgrade: invalid-package"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// specifying non-existant version
func TestDoSystemUpdate_UnavailableVersion(t *testing.T) {
	// Mock tag retrieval with specific versions
	mockTags := []RepositoryTag{
		{Tag: "v1.2.0"},
		{Tag: "v1.3.0"},
	}

	// Store original fetcher and restore after test
	originalFetcher := repoTagsFetcher
	defer func() {
		repoTagsFetcher = originalFetcher
	}()

	repoTagsFetcher = &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version
	tempDir := setupMockVersioning(t, "v1.1.0")
	defer os.RemoveAll(tempDir)

	// Try to upgrade to a version that doesn't exist in upgradable releases
	err := DoSystemUpdate("os", "v2.0.0", nil)
	if err == nil {
		t.Fatal("expected error for unavailable version, got nil")
	}

	expectedError := "release v2.0.0 is not available for os"
	if err.Error() != expectedError {
		t.Errorf("expected error '%s', got '%s'", expectedError, err.Error())
	}
}

// mocking error
func TestDoSystemUpdate_ErrorFromGetUpgradableReleases(t *testing.T) {
	// Store original fetcher and restore after test
	originalFetcher := repoTagsFetcher
	defer func() {
		repoTagsFetcher = originalFetcher
	}()

	// Mock error from repo fetcher
	repoTagsFetcher = &MockRepoTagsFetcher{
		tags: nil,
		err:  fmt.Errorf("network error"),
	}

	err := DoSystemUpdate("os", "v1.2.0", nil)
	if err == nil {
		t.Fatal("expected error from GetUpgradableReleases, got nil")
	}

	if err.Error() != "network error" {
		t.Errorf("expected 'network error', got '%s'", err.Error())
	}
}

func TestStageReleaseFlakeCreatesCloneInConfiguredTempDir(t *testing.T) {
	tmpDir := t.TempDir()
	cloneFunc := func(destination, version string) error {
		return createTestReleaseRepo(t, destination, version)
	}

	stagedPath, commitHash, err := stageReleaseFlakeWithClone(tmpDir, "v1.2.3", nil, cloneFunc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	headHash, err := getRepositoryHeadHash(stagedPath)
	if err == nil {
		t.Fatalf("expected staged release to be a plain path, got readable Git hash %s", headHash)
	}

	expectedCommitHash := stagedCommitHash(t, cloneFunc, "v1.2.3")
	expectedPath := buildStagedReleaseDirPath(tmpDir, "v1.2.3", expectedCommitHash)
	if stagedPath != expectedPath {
		t.Fatalf("expected staged path %q, got %q", expectedPath, stagedPath)
	}
	if commitHash != expectedCommitHash {
		t.Fatalf("expected commit hash %q, got %q", expectedCommitHash, commitHash)
	}

	if _, err := os.Stat(filepath.Join(stagedPath, "flake.nix")); err != nil {
		t.Fatalf("expected staged flake.nix to exist: %v", err)
	}
}

func TestStageReleaseFlakeReplacesExistingHashNamedDir(t *testing.T) {
	tmpDir := t.TempDir()
	cloneFunc := func(destination, version string) error {
		return createTestReleaseRepo(t, destination, version)
	}

	firstStagedPath, _, err := stageReleaseFlakeWithClone(tmpDir, "v1.2.3", nil, cloneFunc)
	if err != nil {
		t.Fatalf("expected first staging run to succeed, got %v", err)
	}

	sentinelPath := filepath.Join(firstStagedPath, "stale.txt")
	if err := os.WriteFile(sentinelPath, []byte("stale"), 0644); err != nil {
		t.Fatalf("failed to create sentinel file: %v", err)
	}

	stagedPath, _, err := stageReleaseFlakeWithClone(tmpDir, "v1.2.3", nil, cloneFunc)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if stagedPath != firstStagedPath {
		t.Fatalf("expected staged path %q, got %q", firstStagedPath, stagedPath)
	}
	if _, err := os.Stat(sentinelPath); !os.IsNotExist(err) {
		t.Fatalf("expected sentinel file to be removed, stat err: %v", err)
	}
}

func TestSystemUpdaterDoSystemUpdateUsesStagedFlakeDir(t *testing.T) {
	originalFetcher := repoTagsFetcher
	defer func() {
		repoTagsFetcher = originalFetcher
	}()

	repoTagsFetcher = &MockRepoTagsFetcher{
		tags: []RepositoryTag{{Tag: "v1.2.0"}},
		err:  nil,
	}

	tempDir := setupMockVersioning(t, "v1.1.0")
	defer os.RemoveAll(tempDir)

	updateTmpDir := t.TempDir()
	var stagedPath string
	var capturedCloneVersion string
	cloneFunc := func(destination, version string) error {
		capturedCloneVersion = version
		if err := createTestReleaseRepo(t, destination, version); err != nil {
			return err
		}

		headHash, err := getRepositoryHeadHash(destination)
		if err != nil {
			return err
		}
		stagedPath = buildStagedReleaseDirPath(updateTmpDir, version, headHash)
		return nil
	}

	var capturedName string
	var capturedArgs []string
	execCommand := func(name string, args ...string) *exec.Cmd {
		capturedName = name
		capturedArgs = append([]string{}, args...)
		return exec.Command("sh", "-c", "exit 0")
	}

	updater := SystemUpdater{
		config: dogeboxd.ServerConfig{
			TmpDir: updateTmpDir,
		},
	}

	if err := doSystemUpdateWithDependencies("os", "v1.2.0", updater.config.TmpDir, nil, cloneFunc, execCommand); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if capturedCloneVersion != "v1.2.0" {
		t.Fatalf("expected clone version %q, got %q", "v1.2.0", capturedCloneVersion)
	}

	if capturedName != SUDO_COMMAND {
		t.Fatalf("expected command %q, got %q", SUDO_COMMAND, capturedName)
	}

	expectedArgs := []string{
		DBXROOT_WRAPPER_COMMAND,
		"nix",
		"rs",
		"--systemd-run",
		"--systemd-unit",
		buildSystemUpdateUnitName("v1.2.0", stagedCommitHash(t, cloneFunc, "v1.2.0")),
		"--flake-dir",
		stagedPath,
		"--cleanup-flake-dir",
		"--set-release",
		"v1.2.0",
	}
	if len(capturedArgs) != len(expectedArgs) {
		t.Fatalf("expected args %v, got %v", expectedArgs, capturedArgs)
	}
	for i, expected := range expectedArgs {
		if capturedArgs[i] != expected {
			t.Fatalf("expected capturedArgs[%d] to be %q, got %q", i, expected, capturedArgs[i])
		}
	}

	if _, err := os.Stat(stagedPath); err != nil {
		t.Fatalf("expected staged flake dir %q to remain available for transient unit cleanup, stat err: %v", stagedPath, err)
	}
}

func createTestReleaseRepo(t *testing.T, destination string, version string) error {
	t.Helper()

	repo, err := git.PlainInit(destination, false)
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(destination, "flake.nix"), []byte("{ }"), 0644); err != nil {
		return err
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return err
	}

	if _, err := worktree.Add("flake.nix"); err != nil {
		return err
	}

	fixedTime := time.Date(2026, time.April, 30, 0, 0, 0, 0, time.UTC)
	_, err = worktree.Commit("test "+version, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  fixedTime,
		},
		Committer: &object.Signature{
			Name:  "Test User",
			Email: "test@example.com",
			When:  fixedTime,
		},
	})
	return err
}

func stagedCommitHash(t *testing.T, cloneFunc func(string, string) error, version string) string {
	t.Helper()

	tmpDir := t.TempDir()
	if err := cloneFunc(tmpDir, version); err != nil {
		t.Fatalf("failed to create test release repo: %v", err)
	}

	hash, err := getRepositoryHeadHash(tmpDir)
	if err != nil {
		t.Fatalf("failed to read test release hash: %v", err)
	}

	return hash
}

/* TODO : check if versioning file(s) were written out
 */

// TestSemverOrderingWithPreReleases verifies that UpgradableReleases are ordered by semver
// including pre-release versions (alpha, beta) and various version formats
func TestSemverOrderingWithPreReleases(t *testing.T) {
	// Create mock tags with various version types including pre-releases
	mockTags := []RepositoryTag{
		{Tag: "v1.5.0"},
		{Tag: "v2.0.0"},
		{Tag: "v1.2.0"},
		{Tag: "v1.10.0"},
		{Tag: "v1.1.0"},
		{Tag: "v2.1.0"},
		{Tag: "v1.0.0"},
		{Tag: "v0.0.1"},
		{Tag: "v0.2.0"},
		{Tag: "v1.0.0-alpha.1"},
		{Tag: "v1.0.0-beta.2"},
		{Tag: "v1.0.0-alpha.2"},
		{Tag: "v2.0.0-beta.1"},
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version to be older than all available versions
	tempDir := setupMockVersioning(t, "v0.0.0")
	defer os.RemoveAll(tempDir)

	releases, err := GetUpgradableReleasesWithFetcher(true, mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return all versions since current is v0.0.0
	expectedCount := 13
	if len(releases) != expectedCount {
		t.Errorf("Expected %d upgradable releases, got %d", expectedCount, len(releases))
	}

	// Verify ordering: highest first, lowest last (including pre-releases)
	expectedOrder := []string{
		"v2.1.0",
		"v2.0.0",
		"v2.0.0-beta.1",
		"v1.10.0",
		"v1.5.0",
		"v1.2.0",
		"v1.1.0",
		"v1.0.0",
		"v1.0.0-beta.2",
		"v1.0.0-alpha.2",
		"v1.0.0-alpha.1",
		"v0.2.0",
		"v0.0.1",
	}

	for i, expectedVersion := range expectedOrder {
		if releases[i].Version != expectedVersion {
			t.Errorf("Expected releases[%d] to be %s, got %s", i, expectedVersion, releases[i].Version)
		}
	}

	// Verify that each version is actually higher than the next one using semver.Compare
	for i := 0; i < len(releases)-1; i++ {
		if semver.Compare(releases[i].Version, releases[i+1].Version) <= 0 {
			t.Errorf("Version ordering incorrect: %s should be higher than %s", releases[i].Version, releases[i+1].Version)
		}
	}

	// Verify specific requested versions are present and in correct positions
	v001Index := -1
	v020Index := -1
	for i, release := range releases {
		if release.Version == "v0.0.1" {
			v001Index = i
		}
		if release.Version == "v0.2.0" {
			v020Index = i
		}
	}

	if v001Index == -1 {
		t.Error("Expected v0.0.1 to be present in releases")
	}
	if v020Index == -1 {
		t.Error("Expected v0.2.0 to be present in releases")
	}

	// Verify v0.2.0 comes before v0.0.1
	if v020Index >= v001Index {
		t.Errorf("Expected v0.2.0 (index %d) to come before v0.0.1 (index %d)", v020Index, v001Index)
	}

	// Verify pre-release versions are handled correctly
	alphaVersions := []string{"v1.0.0-alpha.1", "v1.0.0-alpha.2", "v2.0.0-beta.1", "v1.0.0-beta.2"}
	for _, alphaVersion := range alphaVersions {
		found := false
		for _, release := range releases {
			if release.Version == alphaVersion {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Expected pre-release version %s to be present in releases", alphaVersion)
		}
	}
}

// TestSemverFiltering verifies that invalid semver versions are filtered out
func TestSemverFiltering(t *testing.T) {
	// Create mock tags with valid and invalid semver versions
	// Focus on novel cases not covered by TestSemverValidation
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},                // Valid semver
		{Tag: "v2.0.0"},                // Valid semver
		{Tag: "v3.0"},                  // Valid semver (semver assigns this to v3.0.0)
		{Tag: "bruces-amazing-branch"}, // Invalid semver (branch name)
		{Tag: "latest"},                // Invalid semver (common tag)
		{Tag: "main"},                  // Invalid semver (common tag)
		{Tag: "v1.0.0-alpha.1"},        // Valid semver with pre-release
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version to be older than all valid versions
	tempDir := setupMockVersioning(t, "v0.0.0")
	defer os.RemoveAll(tempDir)

	releases, err := GetUpgradableReleasesWithFetcher(false, mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should only return valid stable semver versions (3 out of 7, excluding pre-releases)
	expectedCount := 3
	if len(releases) != expectedCount {
		t.Errorf("Expected %d valid semver releases, got %d", expectedCount, len(releases))
	}

	// Verify invalid versions are NOT present
	invalidVersions := []string{
		"bruces-amazing-branch",
		"latest",
		"main",
	}

	for _, invalidVersion := range invalidVersions {
		for _, release := range releases {
			if release.Version == invalidVersion {
				t.Errorf("Invalid semver version '%s' should not be present in releases", invalidVersion)
			}
		}
	}

	// Verify all returned versions are valid semver
	for _, release := range releases {
		if !semver.IsValid(release.Version) {
			t.Errorf("Release version '%s' is not a valid semver", release.Version)
		}
	}
}

// TestPreReleaseFiltering verifies that pre-releases can be included or excluded
func TestPreReleaseFiltering(t *testing.T) {
	// Create mock tags with stable and pre-release versions
	mockTags := []RepositoryTag{
		{Tag: "v1.0.0"},         // Stable
		{Tag: "v2.0.0"},         // Stable
		{Tag: "v1.0.0-alpha.1"}, // Pre-release
		{Tag: "v1.0.0-beta.2"},  // Pre-release
		{Tag: "v2.0.0-rc.1"},    // Pre-release
		{Tag: "v3.0"},           // Stable (semver assigns this to v3.0.0)
	}
	mockFetcher := &MockRepoTagsFetcher{tags: mockTags, err: nil}

	// Mock current version to be older than all versions
	tempDir := setupMockVersioning(t, "v0.0.0")
	defer os.RemoveAll(tempDir)

	// Test excluding pre-releases (default behavior)
	releasesWithoutPre, err := GetUpgradableReleasesWithFetcher(false, mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should only return stable versions (3 out of 6)
	expectedStableCount := 3
	if len(releasesWithoutPre) != expectedStableCount {
		t.Errorf("Expected %d stable releases, got %d", expectedStableCount, len(releasesWithoutPre))
	}

	// Verify no pre-releases are present
	for _, release := range releasesWithoutPre {
		if semver.Prerelease(release.Version) != "" {
			t.Errorf("Pre-release version '%s' should not be present when includePreReleases=false", release.Version)
		}
	}

	// Test including pre-releases
	releasesWithPre, err := GetUpgradableReleasesWithFetcher(true, mockFetcher)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	// Should return all versions (6 out of 6)
	expectedAllCount := 6
	if len(releasesWithPre) != expectedAllCount {
		t.Errorf("Expected %d total releases, got %d", expectedAllCount, len(releasesWithPre))
	}

	// Verify pre-releases are present
	preReleaseFound := false
	for _, release := range releasesWithPre {
		if semver.Prerelease(release.Version) != "" {
			preReleaseFound = true
			break
		}
	}
	if !preReleaseFound {
		t.Error("Expected pre-release versions to be present when includePreReleases=true")
	}

	// Verify ordering is still correct (highest first)
	for i := 0; i < len(releasesWithPre)-1; i++ {
		if semver.Compare(releasesWithPre[i].Version, releasesWithPre[i+1].Version) <= 0 {
			t.Errorf("Version ordering incorrect: %s should be higher than %s", releasesWithPre[i].Version, releasesWithPre[i+1].Version)
		}
	}
}
