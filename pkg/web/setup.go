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
	"github.com/dogeorg/dogeboxd/pkg/version"
)

type InitialSystemBootstrapRequestBody struct {
	ReflectorToken string `json:"reflectorToken"`
	ReflectorHost  string `json:"reflectorHost"`
	InitialSSHKey  string `json:"initialSSHKey"`
}

type BootstrapFacts struct {
	HasGeneratedKey                  bool `json:"hasGeneratedKey"`
	HasConfiguredNetwork             bool `json:"hasConfiguredNetwork"`
	HasCompletedInitialConfiguration bool `json:"hasCompletedInitialConfiguration"`
}

type BootstrapResponse struct {
	Version    *version.DBXVersionInfo      `json:"version"`
	DevMode    bool                         `json:"devMode"`
	Assets     map[string]dogeboxd.PupAsset `json:"assets"`
	States     map[string]dogeboxd.PupState `json:"states"`
	Stats      map[string]dogeboxd.PupStats `json:"stats"`
	SetupFacts BootstrapFacts               `json:"setupFacts"`
}

func (t api) getRawBS() BootstrapResponse {
	dbxState := t.sm.Get().Dogebox

	return BootstrapResponse{
		Version: version.GetDBXRelease(),
		DevMode: t.config.DevMode,
		Assets:  t.pups.GetAssetsMap(),
		States:  t.pups.GetStateMap(),
		Stats:   t.pups.GetStatsMap(),
		SetupFacts: BootstrapFacts{
			HasGeneratedKey:                  dbxState.InitialState.HasGeneratedKey,
			HasConfiguredNetwork:             dbxState.InitialState.HasSetNetwork,
			HasCompletedInitialConfiguration: dbxState.InitialState.HasFullyConfigured,
		},
	}
}

type RecoveryFacts struct {
	InstallationMode dogeboxd.BootstrapInstallationMode `json:"installationMode"`
	IsInstalled      bool                               `json:"isInstalled"`
}

type BootstrapRecoveryResponse struct {
	RecoveryFacts RecoveryFacts `json:"recoveryFacts"`
}

func (t api) getRecoveryBS() BootstrapRecoveryResponse {
	dbxState := t.sm.Get().Dogebox

	installationMode, err := system.GetInstallationMode(t.dbx, dbxState)
	if err != nil {
		log.Printf("Could not determine installation mode: %v", err)
		installationMode = dogeboxd.BootstrapInstallationModeCannotInstall
	}
	isInstalled, err := system.IsInstalled(t.dbx, t.config, dbxState)
	if err != nil {
		log.Printf("Could not determine if system is installed: %v", err)
		isInstalled = false
	}

	return BootstrapRecoveryResponse{
		RecoveryFacts: RecoveryFacts{
			InstallationMode: installationMode,
			IsInstalled:      isInstalled,
		},
	}
}

func (t api) getBootstrap(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, t.getRawBS())
}

func (t api) getRecoveryBootstrap(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, t.getRecoveryBS())
}

func (t api) hostReboot(w http.ResponseWriter, r *http.Request) {
	t.lifecycle.Reboot()
}

func (t api) hostShutdown(w http.ResponseWriter, r *http.Request) {
	t.lifecycle.Shutdown()
}

func (t api) getKeymaps(w http.ResponseWriter, r *http.Request) {
	keymaps, err := system.GetKeymaps()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error getting keymaps")
		return
	}

	// Convert keymaps to the desired format
	formattedKeymaps := make([]map[string]string, len(keymaps))
	for i, keymap := range keymaps {
		formattedKeymaps[i] = map[string]string{
			"id":    keymap.Name,
			"label": keymap.Value,
		}
	}

	sendResponse(w, formattedKeymaps)
}

type SetHostnameRequestBody struct {
	Hostname string `json:"hostname"`
}

func (t api) setHostname(w http.ResponseWriter, r *http.Request) {
	dbxState := t.sm.Get().Dogebox

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody SetHostnameRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	dbxState = t.sm.Get().Dogebox
	dbxState.Hostname = requestBody.Hostname

	// TODO: If we've already configured our box, rebuild here?

	if err := t.sm.SetDogebox(dbxState); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error saving state")
		return
	}

	sendResponse(w, map[string]any{"status": "OK"})
}

type SetKeyMapRequestBody struct {
	KeyMap string `json:"keyMap"`
}

func (t api) setKeyMap(w http.ResponseWriter, r *http.Request) {
	dbxState := t.sm.Get().Dogebox

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody SetKeyMapRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	// Fetch available keymaps
	keymaps, err := system.GetKeymaps()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error fetching keymaps")
		return
	}

	// Check if the submitted keymap is valid
	isValidKeymap := false
	for _, keymap := range keymaps {
		if keymap.Name == requestBody.KeyMap {
			isValidKeymap = true
			break
		}
	}

	if !isValidKeymap {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid keymap")
		return
	}

	dbxState = t.sm.Get().Dogebox
	dbxState.KeyMap = requestBody.KeyMap

	// TODO: If we've already configured our box, rebuild here?

	if err := t.sm.SetDogebox(dbxState); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error saving state")
		return
	}

	sendResponse(w, map[string]any{"status": "OK"})
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

	var foundDisk *dogeboxd.SystemDisk

	// Ensure that the provided storage device can actually be used.
	for _, disk := range disks {
		if disk.Name == requestBody.StorageDevice && disk.Suitability.Storage.Usable {
			foundDisk = &disk
			break
		}
	}

	// If the disk selected is actually our boot drive, allow it, and don't set StorageDevice.
	if foundDisk != nil && foundDisk.BootMedia {
		sendResponse(w, map[string]any{"status": "OK"})
		return
	}

	if foundDisk == nil {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid storage device")
		return
	}

	dbxState = t.sm.Get().Dogebox
	dbxState.StorageDevice = requestBody.StorageDevice

	if err := t.sm.SetDogebox(dbxState); err != nil {
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

	if err := t.sm.SetDogebox(dbxState); err != nil {
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

	if err := nixPatch.Apply(); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error initialising system")
		return
	}

	// This storage overlay stuff needs to happen _after_ we've init'd our system, as
	// otherwise we end up in a position where we can't access the $datadir/nix/* files
	// to copy back into our new overlay.. because the overlay is mounted as part of the
	// system init. So we init, copy files, apply overlay, copy files back.
	if dbxState.StorageDevice != "" {
		// Before we do anything, close the DB so we don't have any
		// issues with the overlay mount (ie. stuff not written yet)
		if err := t.sm.CloseDB(); err != nil {
			log.Errf("Error closing DB: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error closing DB")
			return
		}

		tempDir, err := os.MkdirTemp("", "dbx-data-overlay")
		if err != nil {
			log.Errf("Error creating temporary directory: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error creating temporary directory")
			return
		}
		log.Logf("Created temporary directory: %s", tempDir)
		// defer os.RemoveAll(tempDir)

		log.Logf("Initialising storage device: %s", dbxState.StorageDevice)

		partitionName, err := system.InitStorageDevice(dbxState)
		if err != nil {
			log.Errf("Error initialising storage device: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error initialising storage device")
			return
		}

		// Copy all our existing data to our temp dir so we don't lose everything created already.
		if err := utils.CopyFiles(t.config.DataDir, tempDir); err != nil {
			log.Errf("Error copying data to temp dir: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error copying data to temp dir")
			return
		}

		// Apply our new overlay update.
		overlayPatch := t.nix.NewPatch(log)
		t.nix.UpdateStorageOverlay(overlayPatch, partitionName)

		if err := overlayPatch.Apply(); err != nil {
			log.Errf("Error applying overlay patch: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error applying overlay patch")
			return
		}

		// Copy our data back from the temp dir to the new location.
		if err := utils.CopyFiles(tempDir, t.config.DataDir); err != nil {
			log.Errf("Error copying data back to %s: %v", t.config.DataDir, err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error copying data back to data dir")
			return
		}

		// This sucks, but because we wrote our storage-overlay file during the last rebuild,
		// we don't actually have that in the tempDir we backed up. So we have to re-save this
		// file into the overlay we now have mounted, but we don't actually have to rebuild.
		reoverlayPatch := t.nix.NewPatch(log)
		t.nix.UpdateStorageOverlay(reoverlayPatch, partitionName)
		if err := reoverlayPatch.ApplyCustom(dogeboxd.NixPatchApplyOptions{
			DangerousNoRebuild: true,
		}); err != nil {
			log.Errf("Error re-applying overlay patch: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error re-applying overlay patch")
			return
		}

		if err := t.sm.OpenDB(); err != nil {
			log.Errf("Error re-opening store manager: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error re-opening store manager")
			return
		}
	}

	if requestBody.ReflectorToken != "" && requestBody.ReflectorHost != "" {
		if err := system.SaveReflectorTokenForReboot(t.config, requestBody.ReflectorHost, requestBody.ReflectorToken); err != nil {
			log.Errf("Error saving reflector data: %v", err)
		}
	}

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

	dbxs := t.sm.Get().Dogebox
	dbxs.InitialState.HasFullyConfigured = true
	if err := t.sm.SetDogebox(dbxs); err != nil {
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
