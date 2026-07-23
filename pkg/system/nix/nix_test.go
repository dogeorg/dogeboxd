package nix

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestRunManagedRebuildKeepsMarkerForRebuild(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "run", "managed-rebuild-in-progress")

	err := runManagedRebuild(markerPath, func() error {
		info, err := os.Stat(markerPath)
		if err != nil {
			t.Fatalf("managed rebuild marker is unavailable during rebuild: %v", err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("managed rebuild marker mode = %o, want 600", info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("run managed rebuild: %v", err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("managed rebuild marker was not removed: %v", err)
	}
}

func TestRunManagedRebuildRemovesMarkerAfterFailure(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "managed-rebuild-in-progress")
	rebuildErr := errors.New("rebuild failed")

	err := runManagedRebuild(markerPath, func() error { return rebuildErr })
	if !errors.Is(err, rebuildErr) {
		t.Fatalf("run managed rebuild error = %v, want %v", err, rebuildErr)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("managed rebuild marker was not removed after failure: %v", err)
	}
}
