package system

import (
	"os"
	"path/filepath"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func TestMigrateLegacyCustomNixMovesFileIntoDataDir(t *testing.T) {
	dataDir := t.TempDir()
	nixDir := filepath.Join(dataDir, "nix")
	if err := os.MkdirAll(nixDir, 0755); err != nil {
		t.Fatalf("failed to create nix dir: %v", err)
	}

	legacyPath := filepath.Join(nixDir, "custom.nix")
	expectedContent := "{ pkgs, ... }: { }"
	if err := os.WriteFile(legacyPath, []byte(expectedContent), 0644); err != nil {
		t.Fatalf("failed to write legacy custom.nix: %v", err)
	}

	config := dogeboxd.ServerConfig{
		DataDir: dataDir,
		NixDir:  nixDir,
	}

	if err := MigrateLegacyCustomNix(config); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	customNixPath := GetCustomNixPath(config)
	data, err := os.ReadFile(customNixPath)
	if err != nil {
		t.Fatalf("expected migrated custom.nix to exist: %v", err)
	}
	if string(data) != expectedContent {
		t.Fatalf("expected custom.nix content %q, got %q", expectedContent, string(data))
	}
	if _, err := os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy custom.nix to be removed, stat err: %v", err)
	}
}

func TestMoveFileWithVerificationCopiesFileBeforeRemovingSource(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "legacy-custom.nix")
	destinationPath := filepath.Join(tempDir, "data", "custom.nix")
	expectedContent := "{ pkgs, ... }: { services.openssh.enable = true; }"

	if err := os.MkdirAll(filepath.Dir(destinationPath), 0755); err != nil {
		t.Fatalf("failed to create destination dir: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte(expectedContent), 0640); err != nil {
		t.Fatalf("failed to write source custom.nix: %v", err)
	}

	if err := moveFileWithVerification(sourcePath, destinationPath); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	data, err := os.ReadFile(destinationPath)
	if err != nil {
		t.Fatalf("expected destination custom.nix to exist: %v", err)
	}
	if string(data) != expectedContent {
		t.Fatalf("expected custom.nix content %q, got %q", expectedContent, string(data))
	}

	destinationInfo, err := os.Stat(destinationPath)
	if err != nil {
		t.Fatalf("expected destination stat to succeed: %v", err)
	}
	if destinationInfo.Mode().Perm() != 0640 {
		t.Fatalf("expected destination mode %v, got %v", os.FileMode(0640), destinationInfo.Mode().Perm())
	}

	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("expected source custom.nix to be removed, stat err: %v", err)
	}
}

func TestMoveFileWithVerificationReturnsErrorForMissingSource(t *testing.T) {
	tempDir := t.TempDir()
	sourcePath := filepath.Join(tempDir, "legacy-custom.nix")
	destinationPath := filepath.Join(tempDir, "data", "custom.nix")

	if err := moveFileWithVerification(sourcePath, destinationPath); !os.IsNotExist(err) {
		t.Fatalf("expected missing source error, got %v", err)
	}
}
