package dogeboxd

import (
	"encoding/gob"
	"encoding/json"
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
		StatCPU:      NewFloatBuffer(30),
		StatMEM:      NewFloatBuffer(30),
		StatDISK:     NewFloatBuffer(30),
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
	ID           string            `json:"id"`
	Config       map[string]string `json:"config"`
	Installation string            `json:"installation"`
	Status       string            `json:"status"`
	StatCPU      FloatBuffer       `json:"status_cpu"`
	StatMEM      FloatBuffer       `json:"status_mem"`
	StatDISK     FloatBuffer       `json:"status_disk"`
	DevMode      bool              `json:"dev_mode"`
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

type FloatBuffer struct {
	values []float32
	head   int
}

func NewFloatBuffer(size int) FloatBuffer {
	return FloatBuffer{
		values: make([]float32, size),
		head:   0,
	}
}

func (b *FloatBuffer) Add(value float32) {
	b.values[b.head] = value
	b.head = (b.head + 1) % len(b.values)
}

func (b *FloatBuffer) GetValues() []float32 {
	lastN := make([]float32, len(b.values))
	for i := 0; i < len(b.values); i++ {
		lastN[i] = b.values[(b.head-i-1+len(b.values))%len(b.values)]
	}
	return lastN
}

func (b *FloatBuffer) MarshalJSON() ([]byte, error) {
	return json.Marshal(b.GetValues())
}
