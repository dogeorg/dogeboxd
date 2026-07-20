package version

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetDBXReleaseResolvesInstalledPackageSource(t *testing.T) {
	versionDir := t.TempDir()
	packageDir := filepath.Join(versionDir, "dkm")
	sourceDir := filepath.Join(versionDir, "source")
	if err := os.MkdirAll(packageDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sourceDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "rev"), []byte("local-rev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(packageDir, "hash"), []byte("local-hash\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sourceDir, filepath.Join(packageDir, "source")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VERSION_PATH_OVERRIDE", versionDir)

	packageVersion := GetDBXRelease().Packages["dkm"]

	if packageVersion.Source != sourceDir {
		t.Fatalf("expected source %q, got %q", sourceDir, packageVersion.Source)
	}
}
