package web

import (
	"net/http"
	"time"

	"github.com/dogeorg/dogeboxd/pkg/pup"
	"golang.org/x/mod/semver"
)

type StoreListSourceEntryPup struct {
	IsInstalled      bool                       `json:"isInstalled"`
	InstalledID      string                     `json:"installedId"`
	InstalledVersion string                     `json:"installedVersion"`
	LatestVersion    string                     `json:"latestVersion"`
	Versions         map[string]pup.PupManifest `json:"versions"`
}

type StoreListSourceEntry struct {
	LastUpdated string                             `json:"lastUpdated"`
	Pups        map[string]StoreListSourceEntryPup `json:"pups"`
}

func (t api) getStoreList(w http.ResponseWriter, r *http.Request) {
	forceRefresh := r.URL.Query().Get("refresh") == "true"

	available, err := t.sources.GetAll(forceRefresh)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error fetching sources")
		return
	}

	response := map[string]StoreListSourceEntry{}

	for k, source := range available {
		pups := map[string]StoreListSourceEntryPup{}

		// TODO: Ideally not have to do this lookup.
		s, err := t.sources.GetSource(k)
		if err != nil {
			sendErrorResponse(w, http.StatusInternalServerError, "Error fetching source")
			return
		}

		for _, availablePup := range source.Pups {
			// Check if we already have a pup in our list for this version.
			if _, ok := pups[availablePup.Name]; !ok {

				// Check in our pup manager to see if this pup is installed.
				// If it is, we set the InstalledVersion.
				installedPupState := t.dbx.Pups.GetPupFromSource(availablePup.Name, s.Config())

				isInstalled := installedPupState != nil

				var installedVersion string
				var installedID string

				if isInstalled {
					installedVersion = installedPupState.Version
					installedID = installedPupState.ID
				}

				versions := map[string]pup.PupManifest{}

				pups[availablePup.Name] = StoreListSourceEntryPup{
					IsInstalled:      isInstalled,
					InstalledVersion: installedVersion,
					InstalledID:      installedID,
					LatestVersion:    availablePup.Version,
					Versions:         versions,
				}
			}

			// Retrieve the struct, modify it, and store it back in the map
			pupEntry := pups[availablePup.Name]
			pupEntry.Versions[availablePup.Version] = availablePup.Manifest

			if semver.Compare(availablePup.Version, pupEntry.LatestVersion) > 0 {
				pupEntry.LatestVersion = availablePup.Version
			}

			pups[availablePup.Name] = pupEntry
		}

		// Override any entry in the listing with what we actually have installed.
		// We want to show that is _actually_ installed, rather than what might have been removed or updated underneath us.
		// nb. We don't let you remove a source if you have a pup installed from it, so this should be safe here.
		for _, installedPup := range t.dbx.Pups.GetStateMap() {
			if installedPup.Source.Location == s.Config().Location && installedPup.Source.Name == s.Config().Name {
				if _, ok := pups[installedPup.Manifest.Meta.Name]; !ok {
					pups[installedPup.Manifest.Meta.Name] = StoreListSourceEntryPup{
						IsInstalled:      true,
						InstalledVersion: installedPup.Version,
						Versions:         map[string]pup.PupManifest{},
					}
				}

				pups[installedPup.Manifest.Meta.Name].Versions[installedPup.Version] = installedPup.Manifest
			}
		}

		response[k] = StoreListSourceEntry{
			LastUpdated: source.LastUpdated.Format(time.RFC3339),
			Pups:        pups,
		}
	}

	sendResponse(w, response)
}
