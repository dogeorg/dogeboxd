package sources

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

// Implements dogebox.ManifestSource and provides pups
// from the local filesystem, specifically those in development
type LocalFileSource struct {
	dogeboxd.ManifestSourceExport
	path string
}

func NewLocalFileSource(id, label, path string) LocalFileSource {
	s := LocalFileSource{}
	s.ID = id
	s.Label = label
	s.path = path
	s.URL = ""
	s.LastUpdated = time.Now()
	s.Available = []dogeboxd.PupManifest{}

	s.UpdateFromDisk()
	return s
}

func (t LocalFileSource) FindManifestByPupID(id string) (dogeboxd.PupManifest, bool) {
	for _, m := range t.Available {
		if m.ID == id {
			return m, true
		}
	}
	return dogeboxd.PupManifest{}, false
}

// Append or replace available pups
func (t LocalFileSource) UpdateFromDisk() {
	l := findLocalPups(t.path)
	exists := map[string]int{}
	for i, p := range t.Available {
		exists[p.ID] = i
	}

	for _, p := range l {
		p.Hydrate(t.ID)
		fmt.Printf("==== hydrated %s\n", p.ID)
		i, ok := exists[p.ID]
		if ok {
			t.Available = append((t.Available)[:i], (t.Available)[i+1:]...)
		}
		t.Available = append(t.Available, p)
	}
}

func findLocalPups(path string) (pups []dogeboxd.PupManifest) {
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

func loadLocalPupManifest(path string) (man dogeboxd.PupManifest, err error) {
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
