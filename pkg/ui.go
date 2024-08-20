package dogeboxd

import (
	"fmt"
	"log"
	"net/http"
	"path/filepath"
)

func serveSPA(directory string, mainIndex string) http.HandlerFunc {
	mainIndexPath := filepath.Join(directory, mainIndex)

	return func(w http.ResponseWriter, r *http.Request) {
		// Disable caching
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		w.Header().Set("Expires", "0")

		if r.URL.Path == "/" {
			http.ServeFile(w, r, mainIndexPath)
			return
		}

		filePath := filepath.Join(directory, r.URL.Path)

		files, err := filepath.Glob(filePath)
		if err != nil {
			log.Println("Error searching for UI file:", err)
			http.ServeFile(w, r, mainIndexPath)
			return
		}

		// Can't find the requested file, serve index.
		if len(files) == 0 {
			http.ServeFile(w, r, mainIndexPath)
			return
		}

		// Otherwise, serve the requested file
		http.ServeFile(w, r, filePath)
	}
}

func ServeUI(config ServerConfig) {
	entryPoint := "index.html"

	if config.Recovery {
		entryPoint = "index_recovery.html"
		log.Println("In recovery mode: Serving recovery UI")
	} else {
		log.Println("Serving normal UI")
	}

	http.HandleFunc("/", serveSPA(config.UiDir, entryPoint))

	if err := http.ListenAndServe(fmt.Sprintf("%s:%d", config.Bind, config.UiPort), nil); err != nil {
		log.Fatal(err)
	}
}
