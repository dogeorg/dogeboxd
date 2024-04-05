package dogeboxd

import (
	"encoding/gob"
	"encoding/json"
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
