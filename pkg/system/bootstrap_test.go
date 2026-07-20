package system

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMarkInitialBootstrapInProgressCreatesAndClearsMarker(t *testing.T) {
	markerPath := filepath.Join(t.TempDir(), "run", "dogebox", "initial-bootstrap-in-progress")

	clearMarker, err := markInitialBootstrapInProgress(markerPath)
	if err != nil {
		t.Fatalf("mark initial bootstrap in progress: %v", err)
	}
	if _, err := os.Stat(markerPath); err != nil {
		t.Fatalf("expected bootstrap marker to exist: %v", err)
	}

	if err := clearMarker(); err != nil {
		t.Fatalf("clear initial bootstrap marker: %v", err)
	}
	if _, err := os.Stat(markerPath); !os.IsNotExist(err) {
		t.Fatalf("expected bootstrap marker to be removed, got: %v", err)
	}

	if err := clearMarker(); err != nil {
		t.Fatalf("clear missing initial bootstrap marker: %v", err)
	}
}
