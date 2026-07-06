package web

import (
	"log"
	"net/http"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/utils"
	"golang.org/x/mod/semver"
)

type StoreListSourceEntryPup struct {
	LatestVersion    string                          `json:"latestVersion"`
	LogoBase64       string                          `json:"logoBase64"`
	Versions         map[string]dogeboxd.PupManifest `json:"versions"`
	DevModeAvailable bool                            `json:"devModeAvailable"`
}

type StoreListSourceEntry struct {
	Name        string                             `json:"name"`
	Description string                             `json:"description"`
	Location    string                             `json:"location"`
	Type        string                             `json:"type"`
	LastChecked string                             `json:"lastChecked"`
	Pups        map[string]StoreListSourceEntryPup `json:"pups"`
	Error       string                             `json:"error,omitempty"`
}

func (t api) getStoreList(w http.ResponseWriter, r *http.Request) {
	forceRefresh := r.URL.Query().Get("refresh") == "true"

	available, err := t.sources.GetAll(forceRefresh)
	if err != nil {
		log.Println("Error fetching sources:", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Error fetching sources")
		return
	}

	// A manual store refresh should also refresh installed-pup update info
	// so "upgrade available" state is visible without toggling enabled state.
	if forceRefresh {
		jobID := t.dbx.AddAction(dogeboxd.CheckPupUpdates{PupID: ""})
		log.Printf("getStoreList: queued CheckPupUpdates for store refresh (jobID: %s)", jobID)
	}

	response := map[string]StoreListSourceEntry{}

	for k, entry := range available {
		pups := map[string]StoreListSourceEntryPup{}

		for _, availablePup := range entry.Pups {
			// Check if we already have a pup in our list for this version.
			if _, ok := pups[availablePup.Name]; !ok {
				versions := map[string]dogeboxd.PupManifest{}

				isDevModeAvailable := false

				if entry.Config.Type == "disk" {
					devModeServices, err := utils.GetPupNixDevelopmentModeServices(t.config, entry.Config.Location, availablePup.Name, availablePup.Manifest)
					if err != nil {
						log.Println("Error getting dev mode services:", err)
					}

					isDevModeAvailable = len(devModeServices) > 0
				}

				pups[availablePup.Name] = StoreListSourceEntryPup{
					LatestVersion:    availablePup.Version,
					LogoBase64:       availablePup.LogoBase64,
					Versions:         versions,
					DevModeAvailable: isDevModeAvailable,
				}
			}

			// Retrieve the struct, modify it, and store it back in the map
			pupEntry := pups[availablePup.Name]
			pupEntry.Versions[availablePup.Version] = availablePup.Manifest

			if semver.Compare("v"+availablePup.Version, "v"+pupEntry.LatestVersion) > 0 {
				pupEntry.LatestVersion = availablePup.Version
				pupEntry.LogoBase64 = availablePup.LogoBase64
			}

			pups[availablePup.Name] = pupEntry
		}

		response[k] = StoreListSourceEntry{
			Name:        entry.Config.Name,
			Description: entry.Config.Description,
			Location:    entry.Config.Location,
			Type:        entry.Config.Type,
			LastChecked: entry.LastChecked.Format(time.RFC3339),
			Pups:        pups,
			Error:       entry.Error,
		}
	}

	sendResponse(w, response)
}
