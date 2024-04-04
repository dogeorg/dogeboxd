package dogeboxd

import (
	"encoding/gob"
	"fmt"
	"os"
	"path/filepath"
)

func NewPUPStatus(m PupManifest) PupStatus {
	p := PupStatus{
		ID: m.Package,
	}
	return p
}

// PupStatus is persisted to disk
type PupStatus struct {
	ID     string
	Stats  map[string][]float32
	Config map[string]string
	Status string // starting, stopping, running, stopped
}

// Read state from a gob file
func (t PupStatus) Read(dirPath string) error {
	p := filepath.Join(dirPath, fmt.Sprintf("%s_state.gob", t.ID))
	f, err := os.Open(p)
	if err != nil {
		return err
	}
	defer f.Close()

	decoder := gob.NewDecoder(f)
	err = decoder.Decode(&t)
	if err != nil {
		return err
	}

	return nil
}

// write state to a gob file
func (t PupStatus) Write(dirPath string) error {
	p := filepath.Join(dirPath, fmt.Sprintf("%s_state.gob", t.ID))
	f, err := os.OpenFile(p, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}

	defer f.Close()
	encoder := gob.NewEncoder(f)
	encoder.Encode(t)
	return nil
}
