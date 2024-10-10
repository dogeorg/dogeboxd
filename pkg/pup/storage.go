package pup

import (
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// Load all pups from storage
func (t PupManager) loadPups() error {
	// find pup save files
	pupSaveFiles := []string{}
	files, err := os.ReadDir(t.pupDir)
	if err != nil {
		return err
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".gob") {
			pupSaveFiles = append(pupSaveFiles, filepath.Join(t.pupDir, file.Name()))
		}
	}

	for _, path := range pupSaveFiles {
		file, err := os.Open(path)
		if err != nil {
			fmt.Printf("Failed to open pup save file at %q: %v\n", path, err)
			continue
		}
		defer file.Close()

		state := dogeboxd.PupState{}
		decoder := gob.NewDecoder(file)
		if err := decoder.Decode(&state); err != nil {
			if err == io.EOF {
				fmt.Printf("pup state at %q is empty, skipping\n", path)
			}
			fmt.Printf("cannot decode object from file %q: %v", path, err)
			continue
		}

		log.Printf("Loaded pup state: %+v", state)

		// Success! add to index
		t.indexPup(&state)
	}
	return nil
}

/* saves a pup to storage */
func (t PupManager) savePup(p *dogeboxd.PupState) error {
	path := filepath.Join(t.pupDir, fmt.Sprintf("pup_%s.gob", p.ID))
	tempFile, err := os.CreateTemp(t.tmpDir, fmt.Sprintf("temp_%s", p.ID))
	if err != nil {
		return fmt.Errorf("cannot create temporary file: %w", err)
	}
	defer os.Remove(tempFile.Name())

	encoder := gob.NewEncoder(tempFile)
	if err := encoder.Encode(p); err != nil {
		return fmt.Errorf("cannot encode object: %w", err)
	}

	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("cannot close temporary file: %w", err)
	}

	if err := os.Rename(tempFile.Name(), path); err != nil {
		return fmt.Errorf("cannot rename temporary file to %q: %w", path, err)
	}

	t.updateMonitoredPups()
	return nil
}
