package dogeboxd

import (
	"encoding/gob"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func NewPUPStatus(config ServerConfig, m PupManifest) PupStatus {
	f := filepath.Join(config.PupDir, fmt.Sprintf("pup_%s_%s.gob", m.Package, m.Hash))
	p := PupStatus{
		ID:      m.GetID(),
		gobPath: f,
	}
	return p
}

// PupStatus is persisted to disk
type PupStatus struct {
	ID      string
	Stats   map[string][]float32
	Config  map[string]string
	Status  string // starting, stopping, running, stopped
	gobPath string
}

// Read state from a gob file
func (t PupStatus) Read() error {
	file, err := os.Open(t.gobPath)
	if err != nil {
		return fmt.Errorf("cannot open file %q: %w", t.gobPath, err)
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&t); err != nil {
		if err == io.EOF {
			return fmt.Errorf("file %q is empty", t.gobPath)
		}
		return fmt.Errorf("cannot decode object from file %q: %w", t.gobPath, err)
	}
	return nil
}

// write state to a gob file
func (t PupStatus) Write(dirPath string) error {
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

func FindLocalPups(path string) (pups []PupManifest) {
	files, err := os.ReadDir(path)
	if err != nil {
		fmt.Println(err)
		return pups
	}

	for _, file := range files {
		if file.IsDir() {
			subpath := filepath.Join(path, file.Name())
			subFiles, err := os.ReadDir(subpath)
			if err != nil {
				fmt.Println(err)
				return pups
			}

			for _, subFile := range subFiles {
				if subFile.Name() == "pup.json" {
					man, err := loadLocalPupManifest(filepath.Join(subpath, subFile.Name()))
					if err != nil {
						fmt.Println(err)
						continue
					}
					pups = append(pups, man)
					break
				}
			}
		}
	}
	return pups
}

func loadLocalPupManifest(path string) (man PupManifest, err error) {
	file, err := os.Open(path)
	if err != nil {
		fmt.Println(err)
		return
	}
	defer file.Close()

	err = json.NewDecoder(file).Decode(&man)
	if err != nil {
		return man, err
	}
	return man, err
}
