package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
)

type InitialSystemBootstrapRequestBody struct {
	Hostname       string `json:"hostname"`
	ReflectorToken string `json:"reflectorToken"`
	ReflectorHost  string `json:"reflectorHost"`
	InitialSSHKey  string `json:"initialSSHKey"`
}

type BootstrapFacts struct {
	InstallationMode                 dogeboxd.BootstrapInstallationMode `json:"installationMode"`
	HasGeneratedKey                  bool                               `json:"hasGeneratedKey"`
	HasConfiguredNetwork             bool                               `json:"hasConfiguredNetwork"`
	HasCompletedInitialConfiguration bool                               `json:"hasCompletedInitialConfiguration"`
}

type BootstrapResponse struct {
	DevMode    bool                         `json:"devMode"`
	Assets     map[string]dogeboxd.PupAsset `json:"assets"`
	States     map[string]dogeboxd.PupState `json:"states"`
	Stats      map[string]dogeboxd.PupStats `json:"stats"`
	SetupFacts BootstrapFacts               `json:"setupFacts"`
}

func (t api) getRawBS() BootstrapResponse {
	dbxState := t.sm.Get().Dogebox

	installationMode, err := system.GetInstallationMode(dbxState)
	if err != nil {
		log.Printf("Could not determine installation mode: %v", err)
		installationMode = dogeboxd.BootstrapInstallationModeCannotInstall
	}

	return BootstrapResponse{
		DevMode: t.config.DevMode,
		Assets:  t.pups.GetAssetsMap(),
		States:  t.pups.GetStateMap(),
		Stats:   t.pups.GetStatsMap(),
		SetupFacts: BootstrapFacts{
			InstallationMode:                 installationMode,
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
	log := dogeboxd.NewConsoleSubLogger("internal", "initial setup")
	dbxState := t.sm.Get().Dogebox

	if dbxState.InitialState.HasFullyConfigured {
		sendErrorResponse(w, http.StatusForbidden, "System has already been initialised")
		return
	}

	if !dbxState.InitialState.HasGeneratedKey || !dbxState.InitialState.HasSetNetwork {
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

	dbxState.Hostname = requestBody.Hostname
	t.sm.SetDogebox(dbxState)

	if err := t.sm.Save(); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error saving state")
		return
	}

	nixPatch := t.nix.NewPatch(log)

	// This will try and connect to the pending network, and if
	// that works, it will persist the network config to disk properly.
	if err := t.dbx.NetworkManager.TryConnect(nixPatch); err != nil {
		log.Errf("Error connecting to network: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Error connecting to network")
		return
	}

	t.nix.InitSystem(nixPatch, dbxState)

	if err := nixPatch.ApplyCustom(dogeboxd.NixPatchApplyOptions{
		RebuildBoot: true,
	}); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error initialising system")
		return
	}

	if requestBody.ReflectorToken != "" && requestBody.ReflectorHost != "" {
		if err := system.SaveReflectorTokenForReboot(t.config, requestBody.ReflectorHost, requestBody.ReflectorToken); err != nil {
			log.Errf("Error saving reflector data: %v", err)
		}
	}

	dbxs := t.sm.Get().Dogebox
	dbxs.InitialState.HasFullyConfigured = true
	t.sm.SetDogebox(dbxs)

	// Add our DogeOrg source in by default, for people to test things with.
	if _, err := t.sources.AddSource("https://github.com/dogeorg/pups.git"); err != nil {
		log.Errf("Error adding initial dogeorg source: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Error adding dogeorg source")
		return
	}

	// If the user has provided an SSH key, we should add it to the system and enable SSH.
	if requestBody.InitialSSHKey != "" {
		if err := t.dbx.SystemUpdater.AddSSHKey(requestBody.InitialSSHKey, log); err != nil {
			log.Errf("Error adding initial SSH key: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error adding initial SSH key")
			return
		}

		if err := t.dbx.SystemUpdater.EnableSSH(log); err != nil {
			log.Errf("Error enabling SSH: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error enabling SSH")
			return
		}
	}

	if err := t.sm.Save(); err != nil {
		// What should we do here? We've already turned off AP mode so any errors
		// won't get send back to the client. I guess we just reboot?
		// That'll force recovery mode again. We can't even persist this error though.
		sendErrorResponse(w, http.StatusInternalServerError, "Error persisting flags")
	}

	sendResponse(w, map[string]any{"status": "OK"})

	log.Log("Dogebox successfully bootstrapped, rebooting in 5 seconds so we can boot into normal mode.")

	go func() {
		time.Sleep(5 * time.Second)
		t.lifecycle.Reboot()
	}()
}
