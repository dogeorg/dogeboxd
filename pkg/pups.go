package dogeboxd

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func NewPUPStatus(pupDir string, m PupManifest) PupStatus {
	f := filepath.Join(pupDir, fmt.Sprintf("pup_%s.gob", m.Package))
	p := PupStatus{
		ID:           m.ID,
		Installation: "installing",
		Status:       "stopped",
		Stats:        map[string][]float32{"cpu": {1.342, 1.245, 4.123, 2.354}},
		gobPath:      f,
	}
	return p
}

/* Pup status as it relates to nix:
 * ┌─────────────────────────────┬───────────────────────────────┐
 * │Installation                 │  Status                       │
 * ├─────────────────────────────┼───────────────────────────────┤
 * │installing      .nix written │  starting        .nix written │
 * │installed       .nix applied─┼─>started         .nix applied │
 * │broken          .nix failed  │  stopping        .nix written │
 * │                             │  stopped         .nix applied │
 * └─────────────────────────────┴───────────────────────────────┘
 *
 * Valid actions: install, stop, start, restart, uninstall
 */

// PupStatus is persisted to disk
type PupStatus struct {
	ID           string               `json:"id"`
	Stats        map[string][]float32 `json:"stats"`
	Config       map[string]string    `json:"config"`
	Installation string               `json:"installation"`
	Status       string               `json:"status"`
	DevMode      bool                 `json:"dev_mode"`
	gobPath      string
}

// Read state from a gob file
func (t *PupStatus) Read() error {
	file, err := os.Open(t.gobPath)
	if err != nil {
		return fmt.Errorf("cannot open file %q: %w", t.gobPath, err)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(t); err != nil {
		if err == io.EOF {
			return fmt.Errorf("file %q is empty", t.gobPath)
		}
		return fmt.Errorf("cannot decode object from file %q: %w", t.gobPath, err)
	}
	return nil
}

// write state to a gob file
func (t PupStatus) Write() error {
	tempFile, err := os.CreateTemp("", "temp_gob_file")
	if err != nil {
		return fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	encoder := gob.NewEncoder(tempFile)
	if err := encoder.Encode(t); err != nil {
		return fmt.Errorf("cannot encode object: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary file: %w", err)
	}

	if err := os.Rename(tempFile.Name(), t.gobPath); err != nil {
		return fmt.Errorf("cannot rename temporary file to %q: %w", t.gobPath, err)
	}

	return nil
}
