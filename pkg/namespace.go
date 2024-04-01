package dogeboxd

import (
	"fmt"
	"os"
	"path/filepath"
)

func WriteContainerConfig() {
	//
}

var skel []string = []string{
	"/storage",
	"/logs",
	"/tmp",
	"/web/public",
	"/web/local",
}

func EnsureSkel(path string) {
	for _, subpath := range skel {
		fullPath := filepath.Join(path, subpath)
		err := os.MkdirAll(fullPath, os.ModePerm)
		if err != nil {
			fmt.Printf("Failed to create directory %s: %v\n", fullPath, err)
		} else {
			fmt.Printf("Created directory %s\n", fullPath)
		}
	}
}
