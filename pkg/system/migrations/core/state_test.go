package core

import (
	"os"
	"reflect"
	"testing"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

func TestLoadStateMissingFileReturnsEmptyState(t *testing.T) {
	config := dogeboxd.ServerConfig{DataDir: t.TempDir()}

	state, err := LoadState(config)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(state) != 0 {
		t.Fatalf("expected empty state, got %+v", state)
	}
}

func TestSaveStatePreservesUnknownKeys(t *testing.T) {
	config := dogeboxd.ServerConfig{DataDir: t.TempDir()}

	input := State{
		"unknown_migration": {
			RanSuccessfully: true,
			DoNotRun:        true,
			Config: map[string]any{
				"exampleFlag": true,
				"mode":        "test",
			},
		},
	}
	if err := SaveState(config, input); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	state, err := LoadState(config)
	if err != nil {
		t.Fatalf("expected load to succeed, got %v", err)
	}
	if !reflect.DeepEqual(state["unknown_migration"], input["unknown_migration"]) {
		t.Fatalf("expected unknown key preserved, got %+v", state)
	}
}

func TestSetRanSuccessfullyWithoutClearingFlagsOrConfig(t *testing.T) {
	config := dogeboxd.ServerConfig{DataDir: t.TempDir()}

	if err := SaveState(config, State{
		"test_migration": {
			DoNotRun: true,
			Config: map[string]any{
				"exampleFlag": true,
			},
		},
	}); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	if err := SetRanSuccessfully(config, "test_migration", true); err != nil {
		t.Fatalf("expected set ran successfully to succeed, got %v", err)
	}

	state, err := LoadState(config)
	if err != nil {
		t.Fatalf("expected load to succeed, got %v", err)
	}
	record := state["test_migration"]
	if !record.RanSuccessfully || !record.DoNotRun || !record.BoolConfig("exampleFlag") {
		t.Fatalf("expected ranSuccessfully and preserved flags/config, got %+v", record)
	}
}

func TestMarkCompletedSetsTerminalFlagsWithoutClearingConfig(t *testing.T) {
	config := dogeboxd.ServerConfig{DataDir: t.TempDir()}

	if err := SaveState(config, State{
		"test_migration": {
			Config: map[string]any{
				"exampleFlag": true,
			},
		},
	}); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	if err := MarkCompleted(config, "test_migration"); err != nil {
		t.Fatalf("expected mark completed to succeed, got %v", err)
	}

	state, err := LoadState(config)
	if err != nil {
		t.Fatalf("expected load to succeed, got %v", err)
	}
	record := state["test_migration"]
	if !record.RanSuccessfully || !record.DoNotRun || !record.BoolConfig("exampleFlag") {
		t.Fatalf("expected completed flags and preserved config, got %+v", record)
	}
}

func TestSaveStateRoundTripsConfig(t *testing.T) {
	config := dogeboxd.ServerConfig{DataDir: t.TempDir()}

	input := State{
		"test_migration": {
			Config: map[string]any{
				"exampleFlag": true,
			},
		},
	}
	if err := SaveState(config, input); err != nil {
		t.Fatalf("expected save to succeed, got %v", err)
	}

	state, err := LoadState(config)
	if err != nil {
		t.Fatalf("expected load to succeed, got %v", err)
	}
	if !state["test_migration"].BoolConfig("exampleFlag") {
		t.Fatalf("expected config to round-trip, got %+v", state["test_migration"])
	}
}

func TestBoolConfigHandlesMissingAndNonBooleanValues(t *testing.T) {
	record := MigrationRecord{
		Config: map[string]any{
			"enabled": "yes",
		},
	}

	if record.BoolConfig("missing") {
		t.Fatal("expected missing config to be false")
	}
	if record.BoolConfig("enabled") {
		t.Fatal("expected non-boolean config to be false")
	}
}

func TestLoadStateRejectsMalformedJSON(t *testing.T) {
	config := dogeboxd.ServerConfig{DataDir: t.TempDir()}
	if err := os.WriteFile(getStatePath(config), []byte("{invalid"), 0644); err != nil {
		t.Fatalf("expected test file write to succeed, got %v", err)
	}

	if _, err := LoadState(config); err == nil {
		t.Fatal("expected malformed JSON error")
	}
}
