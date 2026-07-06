package utils

import (
	"strings"
	"testing"

	"github.com/Dogebox-WG/dogeboxd/pkg/version"
)

func testVersionInfo() *version.DBXVersionInfo {
	return &version.DBXVersionInfo{
		Release: "v1.0.0",
		Packages: map[string]version.DBXVersionInputTuple{
			"dogeboxd": {Rev: "dogeboxd-rev", Hash: "dogeboxd-hash"},
			"dpanel":   {Rev: "dpanel-rev", Hash: "dpanel-hash"},
			"dkm":      {Rev: "dkm-rev", Hash: "dkm-hash"},
		},
	}
}

func TestGetFlakePathUsesCustomBaseDir(t *testing.T) {
	flakePath := buildFlakePath("/tmp/os-upgrade", "nanopc-t6", "aarch64")
	expected := "/tmp/os-upgrade#dogeboxos-nanopc-t6-aarch64"
	if flakePath != expected {
		t.Fatalf("expected flake path %q, got %q", expected, flakePath)
	}
}

func TestGetRebuildCommandUsesStagedFlakeWithUpgradeOverrides(t *testing.T) {
	command, args, err := buildRebuildCommand("switch", "v9.9.9", "/tmp/os-upgrade#dogeboxos-qemu-x86_64", testVersionInfo())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if command != "nixos-rebuild" {
		t.Fatalf("expected nixos-rebuild command, got %q", command)
	}

	expectedPrefix := []string{
		"switch",
		"--flake",
		"/tmp/os-upgrade#dogeboxos-qemu-x86_64",
		"--impure",
	}
	if len(args) < len(expectedPrefix) {
		t.Fatalf("expected args to start with %v, got %v", expectedPrefix, args)
	}
	for i, expected := range expectedPrefix {
		if args[i] != expected {
			t.Fatalf("expected args[%d] to be %q, got %q", i, expected, args[i])
		}
	}

	joinedArgs := strings.Join(args, " ")
	expectedOverrides := []string{
		"--override-input dogeboxd github:dogebox-wg/dogeboxd/v9.9.9",
		"--override-input dkm github:dogebox-wg/dkm/v9.9.9",
		"--override-input dpanel github:dogebox-wg/dpanel/v9.9.9",
	}
	for _, expected := range expectedOverrides {
		if !strings.Contains(joinedArgs, expected) {
			t.Fatalf("expected staged-flake rebuild args to contain %q, got %q", expected, joinedArgs)
		}
	}
}

func TestGetRebuildCommandUsesVersionOverridesWhenNoFlakeDirIsProvided(t *testing.T) {
	_, args, err := buildRebuildCommand("boot", "v2.0.0", "/etc/nixos#dogeboxos-iso-x86_64", testVersionInfo())
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	joinedArgs := strings.Join(args, " ")
	expectedOverrides := []string{
		"--override-input dogeboxd github:dogebox-wg/dogeboxd/v2.0.0",
		"--override-input dkm github:dogebox-wg/dkm/v2.0.0",
		"--override-input dpanel github:dogebox-wg/dpanel/v2.0.0",
	}
	for _, expected := range expectedOverrides {
		if !strings.Contains(joinedArgs, expected) {
			t.Fatalf("expected rebuild args to contain %q, got %q", expected, joinedArgs)
		}
	}
}
