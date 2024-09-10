package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type InitialSystemBootstrapRequestBody struct {
	ReflectorToken string `json:"reflectorToken"`
}

type BootstrapFacts struct {
	HasGeneratedKey                  bool `json:"hasGeneratedKey"`
	HasConfiguredNetwork             bool `json:"hasConfiguredNetwork"`
	HasCompletedInitialConfiguration bool `json:"hasCompletedInitialConfiguration"`
}

type BootstrapResponse struct {
	States     map[string]dogeboxd.PupState `json:"states"`
	Stats      map[string]dogeboxd.PupStats `json:"stats"`
	SetupFacts BootstrapFacts               `json:"setupFacts"`
}

func (t api) getRawBS() BootstrapResponse {
	dbxState := t.sm.Get().Dogebox

	return BootstrapResponse{
		States: t.pups.GetStateMap(),
		Stats:  t.pups.GetStatsMap(),
		SetupFacts: BootstrapFacts{
			HasGeneratedKey:                  dbxState.InitialState.HasGeneratedKey,
			HasConfiguredNetwork:             dbxState.InitialState.HasSetNetwork,
			HasCompletedInitialConfiguration: dbxState.InitialState.HasFullyConfigured,
		},
	}
}

func (t api) getBootstrap(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, t.getRawBS())
}

func (t api) hostReboot(w http.ResponseWriter, r *http.Request) {
	t.lifecycle.Reboot()
}

func (t api) hostShutdown(w http.ResponseWriter, r *http.Request) {
	t.lifecycle.Shutdown()
}

func (t api) initialBootstrap(w http.ResponseWriter, r *http.Request) {
	// Check a few things first.
	if !t.config.Recovery {
		sendErrorResponse(w, http.StatusForbidden, "Cannot initiate bootstrap in non-recovery mode.")
		return
	}

	dbxis := t.sm.Get().Dogebox.InitialState

	if dbxis.HasFullyConfigured {
		sendErrorResponse(w, http.StatusForbidden, "System has already been initialised")
		return
	}

	if !dbxis.HasGeneratedKey || !dbxis.HasSetNetwork {
		sendErrorResponse(w, http.StatusForbidden, "System not ready to initialise")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody InitialSystemBootstrapRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	// TODO: turn off AP

	// This will try and connect to the pending network, and if
	// that works, it will persist the network config to disk properly.
	if err := t.dbx.NetworkManager.TryConnect(); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error connecting to network")
		return
	}

	if err := t.nix.InitSystem(t.dbx.Pups); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error initialising system")
		return
	}

	if requestBody.ReflectorToken != "" {
		// TODO: ping reflector with relevant internal IP
	}

	dbxs := t.sm.Get().Dogebox
	dbxs.InitialState.HasFullyConfigured = true
	t.sm.SetDogebox(dbxs)

	if err := t.sm.Save(); err != nil {
		// What should we do here? We've already turned off AP mode so any errors
		// won't get send back to the client. I guess we just reboot?
		// That'll force recovery mode again. We can't even persist this error though.
		sendErrorResponse(w, http.StatusInternalServerError, "Error persisting flags")
	}

	sendResponse(w, map[string]any{"status": "OK"})

	// Close the HTTP connection to the client, so we can disconnect successfully.
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported!", http.StatusInternalServerError)
		return
	}

	flusher.Flush()

	if err := t.nix.RebuildBoot(); err != nil {
		log.Printf("Error rebuilding nix: %v", err)
		log.Println("Unfortunately we're going to have to reboot now, and there's no way we can report this to the client.")
		// TODO: Maybe we write a file that gets shown to the user on next boot in dpanel?
	}

	log.Println("Dogebox successfully bootstrapped, rebooting so we can boot into normal mode.")

	if t.config.DevMode {
		log.Printf("In dev mode: Not rebooting, but killing service to make it obvious.")
		os.Exit(0)
		return
	}

	t.lifecycle.Reboot()
}
