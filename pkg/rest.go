package dogeboxd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/dogeorg/dogeboxd/pkg/jwt"
	"github.com/rs/cors"
)

func RESTAPI(config ServerConfig, dbx Dogeboxd, ws WSRelay) conductor.Service {
	a := api{mux: http.NewServeMux(), config: config, dbx: dbx}

	routes := map[string]http.HandlerFunc{
		"GET /bootstrap/": a.getBootstrap,
		//"GET /pup/status":      a.getPupStatus,
		"POST /config/{PupID}": a.updateConfig,
		"POST /auth/login": a.attemptLogin,
		"/ws/state/": ws.GetWSHandler(func() any {
			return Change{ID: "internal", Error: "", Type: "bootstrap", Update: a.getRawBS()}
		}).ServeHTTP,
	}

	for p, h := range routes {
		a.mux.HandleFunc(p, h)
	}

	return a
}

type api struct {
	dbx    Dogeboxd
	mux    *http.ServeMux
	config ServerConfig
}

func (t api) getRawBS() any {
	return map[string]any{
		"manifests": t.dbx.GetManifests(),
		"states":    t.dbx.GetPupStats(),
	}
}

func (t api) getBootstrap(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, t.getRawBS())
}

func (t api) updateConfig(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	data := make(map[string]string)
	err = json.Unmarshal(body, &data)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}
	id := t.dbx.AddAction(UpdatePupConfig{PupID: pupid, Payload: data})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) attemptLogin(w http.ResponseWriter, r *http.Request) {
  // Read the request body
  body, err := ioutil.ReadAll(r.Body)
  if err != nil {
    sendErrorResponse(w, http.StatusBadRequest, "Bad Request")
    return
  }
  // Not done with the body yet.
  defer r.Body.Close()

  // Parse the request body
  var credentials struct { Password string `json:"password"` }
  err = json.Unmarshal(body, &credentials)
  if err != nil {
    sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON, expects { password: 'value' }")
    return
  }

  // Retrieve the stored password from dogeboxd
  dogePupStatus, ok := t.dbx.Pups["internal.dogeboxd"]
	if !ok {
		// TODO: something here
    return
	}

	storedPassword, ok := dogePupStatus.Config["password"]
	if !ok {
    // TODO.  no password, awkward.
    return
	}

  // Check for password match
  if !validatePassword(string(credentials.Password), storedPassword) {
    sendResponse(w, map[string]string{"error": "CHECK-CREDS"})
    return
  }

  // Generate and return a JWT token
  token, err := jwt.GenerateToken(nil /* hmm.. user details? */)
  if err != nil {
    sendErrorResponse(w, http.StatusInternalServerError, "Internal Server Error")
    return
  }

  sendResponse(w, map[string]string{"token": token})
}

// TODO: shift away?
func validatePassword(providedPassword, storedPassword string) bool {
   return providedPassword == storedPassword
}

func (t api) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		handler := cors.Default().Handler(t.mux)
		srv := &http.Server{Addr: fmt.Sprintf("%s:%d", t.config.Bind, t.config.Port), Handler: handler}
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
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // do not cache (Browsers cache GET forever by default)
	w.Write(b)
}

func sendErrorResponse(w http.ResponseWriter, code int, message string) {
	log.Printf("[!] %d: %s\n", code, message)
	// would prefer to use json.Marshal, but this avoids the need
	// to handle encoding errors arising from json.Marshal itself!
	payload := fmt.Sprintf("{\"error\":{\"code\":%q,\"message\":%q}}", code, message)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // do not cache (Browsers cache GET forever by default)
	w.WriteHeader(code)
	w.Write([]byte(payload))
}
