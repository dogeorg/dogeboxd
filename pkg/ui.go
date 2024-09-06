package dogeboxd

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"path/filepath"

	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/rs/cors"
)

type UIServer struct {
	mux    *http.ServeMux
	config ServerConfig
}

func serveSPA(directory string, mainIndex string) http.HandlerFunc {
	mainIndexPath := filepath.Join(directory, mainIndex)

	return func(w http.ResponseWriter, r *http.Request) {
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

func ServeUI(config ServerConfig) conductor.Service {
	entryPoint := "index.html"

	if config.Recovery {
		entryPoint = "index_recovery.html"
		log.Println("In recovery mode: Serving recovery UI")
	} else {
		log.Println("Serving normal UI")
	}

	service := UIServer{
		mux:    http.NewServeMux(),
		config: config,
	}

	service.mux.HandleFunc("/", serveSPA(config.UiDir, entryPoint))

	return service
}

func (t UIServer) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		handler := cors.AllowAll().Handler(t.mux)
		srv := &http.Server{Addr: fmt.Sprintf("%s:%d", t.config.Bind, t.config.UiPort), Handler: handler}
		go func() {
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP server public ListenAndServe: %v", err)
			}
		}()

		started <- true
		ctx := <-stop
		srv.Shutdown(ctx)
		stopped <- true
	}()
	return nil
}
