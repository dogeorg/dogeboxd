package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"golang.org/x/mod/semver"
)

type SystemUpdatePackage struct {
	Name         string                     `json:"name"`
	Updates      []SystemUpdatePackageEntry `json:"updates"`
	LatestUpdate string                     `json:"latestUpdate"`
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

func (t api) getSystemUpdates(w http.ResponseWriter, r *http.Request) {
	upgradableReleases, err := system.GetUpgradableReleases()
	if err != nil {
		log.Printf("Failed to get upgradable releases: %v", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to get upgradable releases")
		return
	}

	packages := map[string]SystemUpdatePackage{
		"dogebox": {
			Name:         "Dogebox",
			Updates:      []SystemUpdatePackageEntry{},
			LatestUpdate: "",
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
