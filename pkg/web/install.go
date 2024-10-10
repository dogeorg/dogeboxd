package web

import (
	"encoding/json"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
)

type InstallToDiskRequest struct {
	Disk   string `json:"disk"`
	Secret string `json:"secret"`
}

func (t api) getInstallDisks(w http.ResponseWriter, r *http.Request) {
	disks, err := system.GetPossibleInstallDisks()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, err.Error())
		return
	}

	sendResponse(w, disks)
}

func (t api) installToDisk(w http.ResponseWriter, r *http.Request) {
	var req InstallToDiskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON: "+err.Error())
		return
	}

	if req.Disk == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Disk is required")
		return
	}

	if req.Secret == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Secret is required")
		return
	}

	if req.Secret != system.DBXRootSecret {
		sendErrorResponse(w, http.StatusForbidden, "Invalid secret")
		return
	}

	dbxState := t.sm.Get().Dogebox

	mode, err := system.GetInstallationMode(dbxState)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Could not determine installation mode")
		return
	}

	if mode != dogeboxd.BootstrapInstallationModeMustInstall && mode != dogeboxd.BootstrapInstallationModeCanInstalled {
		// We're not in a state where we can actually install.
		sendErrorResponse(w, http.StatusBadRequest, "Not in installable state")
	}

	if err := system.InstallToDisk(req.Disk); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error installing to disk: "+err.Error())
		return
	}

	sendResponse(w, map[string]string{"status": "ok"})
}
