package dogeboxd

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/dogeorg/dogeboxd/pkg/conductor"
)

func RESTAPI(config ServerConfig, state State) conductor.Service {
	a := api{mux: http.NewServeMux(), config: config, state: state}

	routes := map[string]http.HandlerFunc{
		"GET /manifest/": a.getManifest,
	}

	for p, h := range routes {
		a.mux.HandleFunc(p, h)
	}

	return a
}

type api struct {
	state  State
	mux    *http.ServeMux
	config ServerConfig
}

func (t api) getManifest(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, t.state.Manifests)
}

func (t api) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		srv := &http.Server{Addr: fmt.Sprintf("%s:%d", t.config.Bind, t.config.Port), Handler: t.mux}
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

// Helpers
func sendResponse(w http.ResponseWriter, payload any) {
	// note: w.Header after this, so we can call sendError
	b, err := json.Marshal(payload)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("in json.Marshal: %s", err.Error()))
		return
	}
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // do not cache (Browsers cache GET forever by default)
	w.Write(b)
}

func sendErrorResponse(w http.ResponseWriter, code int, message string) {
	log.Printf("[!] %d: %s\n", code, message)
	// would prefer to use json.Marshal, but this avoids the need
	// to handle encoding errors arising from json.Marshal itself!
	payload := fmt.Sprintf("{\"error\":{\"code\":%q,\"message\":%q}}", code, message)
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // do not cache (Browsers cache GET forever by default)
	w.WriteHeader(code)
	w.Write([]byte(payload))
}
