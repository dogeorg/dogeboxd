package web

import (
	"log"
	"net/http"
	"time"

	"github.com/dogeorg/dogeboxd/pkg/pup"
	"golang.org/x/mod/semver"
)

type StoreListSourceEntryPup struct {
	LatestVersion string                     `json:"latestVersion"`
	LogoBase64    string                     `json:"logoBase64"`
	Versions      map[string]pup.PupManifest `json:"versions"`
}

type StoreListSourceEntry struct {
	Name        string                             `json:"name"`
	Description string                             `json:"description"`
	Location    string                             `json:"location"`
	Type        string                             `json:"type"`
	LastChecked string                             `json:"lastChecked"`
	Pups        map[string]StoreListSourceEntryPup `json:"pups"`
}

func (t api) getStoreList(w http.ResponseWriter, r *http.Request) {
	forceRefresh := r.URL.Query().Get("refresh") == "true"

	available, err := t.sources.GetAll(forceRefresh)
	if err != nil {
		log.Println("Error fetching sources:", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Error fetching sources")
		return
	}

	response := map[string]StoreListSourceEntry{}

	for k, entry := range available {
		pups := map[string]StoreListSourceEntryPup{}

		for _, availablePup := range entry.Pups {
			// Check if we already have a pup in our list for this version.
			if _, ok := pups[availablePup.Name]; !ok {
				versions := map[string]pup.PupManifest{}

				pups[availablePup.Name] = StoreListSourceEntryPup{
					LatestVersion: availablePup.Version,
					LogoBase64:    availablePup.LogoBase64,
					Versions:      versions,
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

		// Override any entry in the listing with what we actually have installed.
		// We want to show that is _actually_ installed, rather than what might have been removed or updated underneath us.
		// nb. We don't let you remove a source if you have a pup installed from it, so this should be safe here.
		for _, installedPup := range t.dbx.Pups.GetStateMap() {
			if installedPup.Source.Location == entry.Config.Location && installedPup.Source.Name == entry.Config.Name {
				if _, ok := pups[installedPup.Manifest.Meta.Name]; !ok {
					pups[installedPup.Manifest.Meta.Name] = StoreListSourceEntryPup{
						Versions: map[string]pup.PupManifest{},
					}
				}

				pups[installedPup.Manifest.Meta.Name].Versions[installedPup.Version] = installedPup.Manifest
			}
		}

		response[k] = StoreListSourceEntry{
			Name:        entry.Config.Name,
			Description: entry.Config.Description,
			Location:    entry.Config.Location,
			Type:        entry.Config.Type,
			LastChecked: entry.LastChecked.Format(time.RFC3339),
			Pups:        pups,
		}
	}

	sendResponse(w, response)
}
