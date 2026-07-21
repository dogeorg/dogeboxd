package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/version"
)

type InitialSystemBootstrapRequestBody struct {
	ReflectorToken              string `json:"reflectorToken"`
	ReflectorHost               string `json:"reflectorHost"`
	InitialSSHKey               string `json:"initialSSHKey"`
	UseFoundationOSBinaryCache  bool   `json:"useFoundationOSBinaryCache"`
	UseFoundationPupBinaryCache bool   `json:"useFoundationPupBinaryCache"`
}

type BootstrapFacts struct {
	HasGeneratedKey                  bool   `json:"hasGeneratedKey"`
	HasConfiguredNetwork             bool   `json:"hasConfiguredNetwork"`
	HasCompletedInitialConfiguration bool   `json:"hasCompletedInitialConfiguration"`
	SetupSessionID                   string `json:"setupSessionId"`
	ActiveBootstrapJobId             string `json:"activeBootstrapJobId,omitempty"`
	ActiveSystemUpdateJobId          string `json:"activeSystemUpdateJobId,omitempty"`
	ActiveSystemUpdateStatus         string `json:"activeSystemUpdateStatus,omitempty"`
}

type BootstrapFlags struct {
	IsFirstTimeWelcomeComplete bool `json:"isFirstTimeWelcomeComplete"`
	IsDeveloperMode            bool `json:"isDeveloperMode"`
}

type SidebarPreferencesResponse struct {
	SidebarPups []string `json:"sidebarPups"`
}

type BootstrapResponse struct {
	TS                 int64                        `json:"ts"`
	Version            *version.DBXVersionInfo      `json:"version"`
	DevMode            bool                         `json:"devMode"`
	Assets             map[string]dogeboxd.PupAsset `json:"assets"`
	States             map[string]dogeboxd.PupState `json:"states"`
	Stats              map[string]dogeboxd.PupStats `json:"stats"`
	Flags              BootstrapFlags               `json:"flags"`
	SetupFacts         BootstrapFacts               `json:"setupFacts"`
	SidebarPreferences SidebarPreferencesResponse   `json:"sidebarPreferences"`
}

func (t api) getRawBS() BootstrapResponse {
	dbxState := t.sm.Get().Dogebox
	activeBootstrapJobID := ""
	activeSystemUpdateJobID := ""
	activeSystemUpdateStatus := ""

	if t.dbx.JobManager != nil {
		activeJobs, err := t.dbx.JobManager.GetActiveJobs()
		if err != nil {
			log.Printf("Could not determine active bootstrap job: %v", err)
		} else {
			for _, job := range activeJobs {
				if job.Action == "initial-bootstrap" {
					activeBootstrapJobID = job.ID
				}
				if job.Action == "system-update" {
					activeSystemUpdateJobID = job.ID
					activeSystemUpdateStatus = string(job.Status)
				}
			}
		}
	}

	// Get sidebar pups from DogeboxState, ensuring non-nil slice for JSON
	sidebarPups := dbxState.SidebarPups
	if sidebarPups == nil {
		sidebarPups = []string{}
	}

	return BootstrapResponse{
		TS:      time.Now().UnixMilli(),
		Version: version.GetDBXRelease(),
		DevMode: t.config.DevMode,
		Assets:  t.pups.GetAssetsMap(),
		States:  t.pups.GetStateMap(),
		Stats:   t.pups.GetStatsMap(),
		Flags: BootstrapFlags{
			IsFirstTimeWelcomeComplete: dbxState.Flags.IsFirstTimeWelcomeComplete,
			IsDeveloperMode:            dbxState.Flags.IsDeveloperMode,
		},
		SetupFacts: BootstrapFacts{
			HasGeneratedKey:                  dbxState.InitialState.HasGeneratedKey,
			HasConfiguredNetwork:             dbxState.InitialState.HasSetNetwork,
			HasCompletedInitialConfiguration: dbxState.InitialState.HasFullyConfigured,
			SetupSessionID:                   dbxState.InitialState.SetupSessionID,
			ActiveBootstrapJobId:             activeBootstrapJobID,
			ActiveSystemUpdateJobId:          activeSystemUpdateJobID,
			ActiveSystemUpdateStatus:         activeSystemUpdateStatus,
		},
		SidebarPreferences: SidebarPreferencesResponse{SidebarPups: sidebarPups},
	}
}

type RecoveryFacts struct {
	InstallationBootMedia dogeboxd.BootstrapInstallationBootMedia `json:"installationBootMedia"`
	InstallationState     dogeboxd.BootstrapInstallationState     `json:"installationState"`
}

type BootstrapRecoveryResponse struct {
	RecoveryFacts RecoveryFacts `json:"recoveryFacts"`
}

func (t api) getRecoveryBS() BootstrapRecoveryResponse {
	dbxState := t.sm.Get().Dogebox

	installationMedia, installationState, err := system.GetInstallationState(t.dbx, t.config, dbxState)
	if err != nil {
		log.Printf("Could not determine installation mode: %v", err)
		installationState = dogeboxd.BootstrapInstallationStateNotInstalled
	}

	return BootstrapRecoveryResponse{
		RecoveryFacts: RecoveryFacts{
			InstallationBootMedia: installationMedia,
			InstallationState:     installationState,
		},
	}
}

func (t api) getBootstrap(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, t.getRawBS())
}

func (t api) getRecoveryBootstrap(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, t.getRecoveryBS())
}

func (t api) setWelcomeComplete(w http.ResponseWriter, r *http.Request) {
	dbxState := t.sm.Get().Dogebox
	dbxState.Flags.IsFirstTimeWelcomeComplete = true

	if err := t.sm.SetDogebox(dbxState); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error saving state")
		return
	}

	sendResponse(w, map[string]any{"status": "OK"})
}

type InstallPupCollectionRequest struct {
	CollectionName string `json:"collectionName"`
}

func (t api) installPupCollection(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody InstallPupCollectionRequest
	if err := json.Unmarshal(body, &requestBody); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error parsing payload")
		return
	}

	if requestBody.CollectionName == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Collection name is required")
		return
	}

	// Get the session token for authentication
	session, sessionOK := getSession(r, getBearerToken)
	if !sessionOK {
		sendErrorResponse(w, http.StatusUnauthorized, "Failed to fetch session")
		return
	}

	// Process the collection installation
	processPupCollections(t.sources, t.dbx, session.DKM_TOKEN, requestBody.CollectionName)

	sendResponse(w, map[string]any{"success": true})
}

func (t api) hostReboot(w http.ResponseWriter, r *http.Request) {
	t.lifecycle.Reboot()
}

func (t api) hostShutdown(w http.ResponseWriter, r *http.Request) {
	t.lifecycle.Shutdown()
}

func (t api) getKeymap(w http.ResponseWriter, r *http.Request) {
	keymap, err := t.nix.GetConfigValue("console.keyMap")
	keymap = strings.TrimSpace(keymap)
	if err != nil || keymap == "" {
		if fb := strings.TrimSpace(t.sm.Get().Dogebox.KeyMap); fb != "" {
			sendResponse(w, fb)
			return
		}
		if err != nil {
			log.Printf("getKeymap: nix eval failed: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error getting current keymap")
			return
		}
	}

	sendResponse(w, keymap)
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

func (t api) getTimezone(w http.ResponseWriter, r *http.Request) {
	timezone, err := t.nix.GetConfigValue("time.timeZone")
	timezone = strings.TrimSpace(timezone)
	if err != nil || timezone == "" {
		if fb := strings.TrimSpace(t.sm.Get().Dogebox.Timezone); fb != "" {
			sendResponse(w, fb)
			return
		}
		if err != nil {
			log.Printf("getTimezone: nix eval failed: %v", err)
			sendErrorResponse(w, http.StatusInternalServerError, "Error getting current timezone")
			return
		}
	}

	sendResponse(w, timezone)
}

func (t api) getTimezones(w http.ResponseWriter, r *http.Request) {
	timezones, err := system.GetTimezones()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error getting timezones")
		return
	}

	// Convert timezones to the desired format
	formattedTimezones := make([]map[string]string, len(timezones))
	for i, timezone := range timezones {
		formattedTimezones[i] = map[string]string{
			"id":    timezone.Name,
			"label": timezone.Value,
		}
	}

	sendResponse(w, formattedTimezones)
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

	// TODO : should rebuild actually be here?
	if dbxState.InitialState.HasFullyConfigured {
		action := dogeboxd.UpdateKeymap{Keymap: requestBody.KeyMap}
		id := t.dbx.AddAction(action)
		sendResponse(w, map[string]any{"status": "OK", "id": id})
		return
	}

	sendResponse(w, map[string]any{"status": "OK"})
}

type SetTimezoneRequestBody struct {
	Timezone string `json:"timezone"`
}

func (t api) setTimezone(w http.ResponseWriter, r *http.Request) {
	dbxState := t.sm.Get().Dogebox

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody SetTimezoneRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	// Fetch available timezones
	timezones, err := system.GetTimezones()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error fetching timezones")
		return
	}

	// Check if the submitted timezone is valid
	isValidTimezone := false
	for _, timezone := range timezones {
		if timezone.Name == requestBody.Timezone {
			isValidTimezone = true
			break
		}
	}

	if !isValidTimezone {
		sendErrorResponse(w, http.StatusBadRequest, "Invalid timezone")
		return
	}

	dbxState = t.sm.Get().Dogebox
	dbxState.Timezone = requestBody.Timezone

	// TODO: If we've already configured our box, rebuild here?

	if err := t.sm.SetDogebox(dbxState); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error saving state")
		return
	}

	// TODO : should rebuild actually be here?
	if dbxState.InitialState.HasFullyConfigured {
		action := dogeboxd.UpdateTimezone{Timezone: requestBody.Timezone}
		id := t.dbx.AddAction(action)
		sendResponse(w, map[string]any{"status": "OK", "id": id})
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

	id := t.dbx.AddAction(dogeboxd.InitialBootstrap{
		ReflectorToken:              requestBody.ReflectorToken,
		ReflectorHost:               requestBody.ReflectorHost,
		InitialSSHKey:               requestBody.InitialSSHKey,
		UseFoundationOSBinaryCache:  requestBody.UseFoundationOSBinaryCache,
		UseFoundationPupBinaryCache: requestBody.UseFoundationPupBinaryCache,
	})

	sendResponse(w, map[string]any{"jobId": id})
}

// getSidebarPreferences returns the list of pups pinned to the sidebar
func (t api) getSidebarPreferences(w http.ResponseWriter, r *http.Request) {
	dbxState := t.sm.Get().Dogebox
	sidebarPups := dbxState.SidebarPups
	if sidebarPups == nil {
		sidebarPups = []string{}
	}
	sendResponse(w, SidebarPreferencesResponse{SidebarPups: sidebarPups})
}

// addSidebarPup adds a pup to the sidebar
func (t api) addSidebarPup(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var payload struct {
		PupID string `json:"pupId"`
	}
	err = json.Unmarshal(body, &payload)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	if payload.PupID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "pupId is required")
		return
	}

	dbxState := t.sm.Get().Dogebox
	sidebarPups := dbxState.SidebarPups
	if sidebarPups == nil {
		sidebarPups = []string{}
	}

	// Check if already exists
	for _, id := range sidebarPups {
		if id == payload.PupID {
			sendResponse(w, map[string]string{"status": "OK", "message": "Already in sidebar"})
			return
		}
	}

	// Add to list and persist
	dbxState.SidebarPups = append(sidebarPups, payload.PupID)
	err = t.sm.SetDogebox(dbxState)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error saving preferences")
		return
	}

	sendResponse(w, map[string]string{"status": "OK"})
}

// removeSidebarPup removes a pup from the sidebar
func (t api) removeSidebarPup(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var payload struct {
		PupID string `json:"pupId"`
	}
	err = json.Unmarshal(body, &payload)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	if payload.PupID == "" {
		sendErrorResponse(w, http.StatusBadRequest, "pupId is required")
		return
	}

	dbxState := t.sm.Get().Dogebox
	sidebarPups := dbxState.SidebarPups
	if sidebarPups == nil {
		// Nothing to remove
		sendResponse(w, map[string]string{"status": "OK", "message": "Not in sidebar"})
		return
	}

	// Remove from list
	filtered := []string{}
	found := false
	for _, id := range sidebarPups {
		if id != payload.PupID {
			filtered = append(filtered, id)
		} else {
			found = true
		}
	}

	if !found {
		sendResponse(w, map[string]string{"status": "OK", "message": "Not in sidebar"})
		return
	}

	// Persist the updated list
	dbxState.SidebarPups = filtered
	err = t.sm.SetDogebox(dbxState)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error saving preferences")
		return
	}

	sendResponse(w, map[string]string{"status": "OK"})
}
