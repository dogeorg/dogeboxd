package web

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"time"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/dogeorg/dogeboxd/pkg/version"
	"golang.org/x/mod/semver"
)

type SystemUpdatePackage struct {
	Name           string                     `json:"name"`
	Updates        []SystemUpdatePackageEntry `json:"updates"`
	CurrentVersion string                     `json:"currentVersion"`
	LatestUpdate   string                     `json:"latestUpdate"`
}

type SystemUpdatePackageEntry struct {
	Version    string `json:"version"`
	Summary    string `json:"summary"`
	ReleaseURL string `json:"releaseURL"`
}

type GetSystemUpdatesResponse struct {
	Packages map[string]SystemUpdatePackage `json:"packages"`
}

type DoSystemUpdateRequest struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

func GetSystemUpdates(dbx *dogeboxd.Dogeboxd) (map[string]SystemUpdatePackage, error) {
	upgradableReleases, err := system.GetUpgradableReleases()
	if err != nil {
		log.Printf("Failed to get upgradable releases: %v", err)
		return nil, err
	}

	dbxRelease := version.GetDBXRelease()

	packages := map[string]SystemUpdatePackage{
		"dogebox": {
			Name:           "Dogebox",
			Updates:        []SystemUpdatePackageEntry{},
			CurrentVersion: dbxRelease.Release,
			LatestUpdate:   "",
		},
	}

	for _, release := range upgradableReleases {
		pkg := packages["dogebox"]
		entry := SystemUpdatePackageEntry{
			Version:    release.Version,
			Summary:    release.Summary,
			ReleaseURL: release.ReleaseURL,
		}
		pkg.Updates = append(pkg.Updates, entry)

		if pkg.LatestUpdate == "" || semver.Compare(release.Version, pkg.LatestUpdate) > 0 {
			pkg.LatestUpdate = release.Version
		}

		packages["dogebox"] = pkg
	}

	// Check if any of the available packages have an update available.
	for _, pkg := range packages {
		if pkg.LatestUpdate != pkg.CurrentVersion {
			dbx.SendSystemUpdateAvailable()
		}
	}

	return packages, nil
}

func (t api) getSystemUpdatesForWeb(w http.ResponseWriter, r *http.Request) {
	packages, err := GetSystemUpdates(&t.dbx)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to get upgradable releases")
		return
	}

	sendResponse(w, GetSystemUpdatesResponse{
		Packages: packages,
	})
}

func (t api) doSystemUpdate(w http.ResponseWriter, r *http.Request) {
	var req DoSystemUpdateRequest
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	if req.Package == "" || req.Version == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Package and version are required")
		return
	}

	update := dogeboxd.SystemUpdateRequest{
		Package: req.Package,
		Version: req.Version,
	}

	id := t.dbx.AddAction(update)
	sendResponse(w, map[string]string{"id": id})
}

type UpdateChecker struct {
	dbx *dogeboxd.Dogeboxd
}

func NewUpdateChecker(dbx *dogeboxd.Dogeboxd) conductor.Service {
	return &UpdateChecker{
		dbx: dbx,
	}
}

func (uc *UpdateChecker) Run(started, stopped chan bool, stop chan context.Context) error {
	checker := func() {
		ticker := time.NewTicker(6 * time.Hour)
		defer ticker.Stop()

		for {
			log.Println("Checking for system updates")
			GetSystemUpdates(uc.dbx)
			<-ticker.C
		}
	}

	go func() {
		started <- true

		time.Sleep(30 * time.Second)

		_, err := GetSystemUpdates(uc.dbx)
		if err != nil {
			log.Printf("Failed to get system upgrades: %v", err)
		}

		// Start checking every now and again.
		go checker()

		<-stop
		stopped <- true
	}()

	return nil
}
