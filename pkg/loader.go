package dogeboxd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

const pupTpl string = `
Package: {{.Package}} 
Hash: {{.Hash}}
Fields:

{{ range .Command.Config.Sections }}
	+ Section {{.Label}}
	{{ range .Fields }}
		- {{.name}}
	{{ end }}
{{ end }}

`

func PrintPups(pups []PupManifest) {
	tmpl, err := template.New("puptpl").Parse(pupTpl)
	if err != nil {
		fmt.Println(err)
		return
	}

	for _, pup := range pups {
		err = tmpl.Execute(os.Stdout, pup)
		if err != nil {
			fmt.Println(err)
		}
	}
}

func FlattenPups(pups []PupManifest) []byte {
	data, err := json.Marshal(pups)
	if err != nil {
		fmt.Println("jandle me")
	}
	return data
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
					man, err := LoadManifest(filepath.Join(subpath, subFile.Name()))
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

func LoadManifest(path string) (man PupManifest, err error) {
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