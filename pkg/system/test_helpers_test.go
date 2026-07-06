package system

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// AssertUpgradableRelease checks if an upgradable release has expected properties
func AssertUpgradableRelease(t *testing.T, release UpgradableRelease, expectedVersion string) {
	if release.Version != expectedVersion {
		t.Errorf("Expected version %s, got %s", expectedVersion, release.Version)
	}

	expectedURL := "https://github.com/dogebox-wg/os/releases/tag/" + expectedVersion
	if release.ReleaseURL != expectedURL {
		t.Errorf("Expected release URL %s, got %s", expectedURL, release.ReleaseURL)
	}

	expectedSummary := "Update for Dogeboxd / DKM / DPanel"
	if release.Summary != expectedSummary {
		t.Errorf("Expected summary '%s', got '%s'", expectedSummary, release.Summary)
	}
}

// Common test data generators
func GenerateSequentialVersions(major, minor int, count int) []RepositoryTag {
	tags := make([]RepositoryTag, count)
	for i := 0; i < count; i++ {
		tags[i] = RepositoryTag{Tag: fmt.Sprintf("v%d.%d.%d", major, minor+i, 0)}
	}
	return tags
}

// Error scenarios for testing
var (
	ErrNetworkTimeout  = fmt.Errorf("network timeout")
	ErrUnauthorized    = fmt.Errorf("unauthorized access")
	ErrRepoNotFound    = fmt.Errorf("repository not found")
	ErrInvalidResponse = fmt.Errorf("invalid response format")
)

// CreateErrorMock creates a mock that returns an error
func CreateErrorMock(err error) *MockRepoTagsFetcher {
	return &MockRepoTagsFetcher{
		tags: nil,
		err:  err,
	}
}

// CreateSuccessMock creates a mock that returns specific tags
func CreateSuccessMock(tags []RepositoryTag) *MockRepoTagsFetcher {
	return &MockRepoTagsFetcher{
		tags: tags,
		err:  nil,
	}
}

// TableDrivenTest represents a test case for table-driven testing
type TableDrivenTest struct {
	Name             string
	CurrentVersion   string
	AvailableTags    []RepositoryTag
	MockError        error
	ExpectedCount    int
	ExpectedError    string
	ExpectedVersions []string
}

// executes a series of table-driven tests for GetUpgradableReleases
func RunTableDrivenTests(t *testing.T, tests []TableDrivenTest) {
	for _, tt := range tests {
		t.Run(tt.Name, func(t *testing.T) {
			// Setup mock
			mockFetcher := &MockRepoTagsFetcher{
				tags: tt.AvailableTags,
				err:  tt.MockError,
			}

			// Mock version
			tempDir := setupMockVersioning(t, tt.CurrentVersion)
			defer os.RemoveAll(tempDir)

			releases, err := GetUpgradableReleasesWithFetcher(false, mockFetcher)

			// Check error expectations
			if tt.ExpectedError != "" {
				if err == nil {
					t.Fatalf("Expected error '%s', got nil", tt.ExpectedError)
				}
				if err.Error() != tt.ExpectedError {
					t.Errorf("Expected error '%s', got '%s'", tt.ExpectedError, err.Error())
				}
				return
			}

			// Check success expectations
			if err != nil {
				t.Fatalf("Unexpected error: %v", err)
			}

			if len(releases) != tt.ExpectedCount {
				t.Errorf("Expected %d releases, got %d", tt.ExpectedCount, len(releases))
			}

			// Check specific versions if provided
			if len(tt.ExpectedVersions) > 0 {
				if len(releases) != len(tt.ExpectedVersions) {
					t.Errorf("Expected %d versions, got %d", len(tt.ExpectedVersions), len(releases))
				}

				for i, expectedVersion := range tt.ExpectedVersions {
					if i >= len(releases) {
						t.Errorf("Missing expected version %s at index %d", expectedVersion, i)
						continue
					}
					AssertUpgradableRelease(t, releases[i], expectedVersion)
				}
			}
		})
	}
}

// setupMockVersioning creates a temporary version directory for testing
func setupMockVersioning(t testing.TB, release string) string {
	tempDir, err := os.MkdirTemp("", "version_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp directory: %v", err)
	}

	versionDir := filepath.Join(tempDir, "versioning")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to create versioning directory: %v", err)
	}

	if err := os.WriteFile(filepath.Join(versionDir, "dbx"), []byte(release), 0644); err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("Failed to write version file: %v", err)
	}

	// Temporarily change the version lookup path to our temp directory
	originalPath := os.Getenv("VERSION_PATH_OVERRIDE")
	os.Setenv("VERSION_PATH_OVERRIDE", versionDir)

	// Restore original path when test completes
	t.Cleanup(func() {
		if originalPath == "" {
			os.Unsetenv("VERSION_PATH_OVERRIDE")
		} else {
			os.Setenv("VERSION_PATH_OVERRIDE", originalPath)
		}
	})

	return tempDir
}
