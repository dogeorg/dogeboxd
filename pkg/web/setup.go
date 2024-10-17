package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/dogeorg/dogeboxd/pkg/utils"
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

type SetStorageDeviceRequestBody struct {
	StorageDevice string `json:"storageDevice"`
}

func (t api) setStorageDevice(w http.ResponseWriter, r *http.Request) {
	dbxState := t.sm.Get().Dogebox

	if dbxState.InitialState.HasFullyConfigured {
		sendErrorResponse(w, http.StatusForbidden, "Cannot set storage device once initial setup has completed")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody SetStorageDeviceRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	disks, err := system.GetSystemDisks()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error getting system disks")
		return
	}

	diskOK := false

	// Ensure that the provided storage device can actually be used.
	for _, disk := range disks {
		if disk.Name == requestBody.StorageDevice && disk.SuitableDataDrive {
			diskOK = true
			break
		}
	}

	if !diskOK {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid storage device")
		return
	}

	dbxState = t.sm.Get().Dogebox
	dbxState.StorageDevice = requestBody.StorageDevice
	t.sm.SetDogebox(dbxState)

	if err := t.sm.Save(); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error saving state")
		return
	}

	sendResponse(w, map[string]any{"status": "OK"})
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

	// Create a temporary directory that is used as the target location to copy our
	// data to if we have a separate storage device that we will use as an overlay for /opt.
	tempDir, err := os.MkdirTemp("", "dbx-data")
	if err != nil {
		log.Errf("Error creating temporary directory: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Error creating temporary directory")
		return
	}
	defer os.RemoveAll(tempDir)

	if dbxState.StorageDevice != "" {
		log.Logf("Initialising storage device: %s", dbxState.StorageDevice)

		partitionName, err := system.InitStorageDevice(dbxState)
		if err != nil {
			log.Errf("Error initialising storage device: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error initialising storage device")
			return
		}

		t.nix.UpdateStorageOverlay(nixPatch, partitionName)

		// Copy all our existing data to our temp dir so we don't lose everything created already.
		if err := utils.CopyFiles(t.config.DataDir, tempDir); err != nil {
			log.Errf("Error copying data to temp dir: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error copying data to temp dir")
			return
		}
	}

	t.nix.InitSystem(nixPatch, dbxState)

	rebuildBoot := true

	if t.config.DevMode {
		// Mostly safe to rebuild-switch if we're in dev mode.
		// This will make sure that our device is mounted right now.
		rebuildBoot = false
	}

	if err := nixPatch.ApplyCustom(dogeboxd.NixPatchApplyOptions{
		RebuildBoot: rebuildBoot,
	}); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error initialising system")
		return
	}

	if requestBody.ReflectorToken != "" && requestBody.ReflectorHost != "" {
		if err := system.SaveReflectorTokenForReboot(t.config, requestBody.ReflectorHost, requestBody.ReflectorToken); err != nil {
			log.Errf("Error saving reflector data: %v", err)
		}
	}

	// At this point, if we have a specified storage device, it should already be mounted over /opt.
	// So we copy our data back from the temp dir to /opt.
	if dbxState.StorageDevice != "" {
		if err := utils.CopyFiles(tempDir, t.config.DataDir); err != nil {
			log.Errf("Error copying data back to /opt: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error copying data back to /opt")
			return
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
