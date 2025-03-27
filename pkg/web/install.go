package web

import (
	"encoding/json"
	"net/http"

	"github.com/dogeorg/dogeboxd/pkg/system"
)

type InstallToDiskRequest struct {
	Disk   string `json:"disk"`
	Secret string `json:"secret"`
}

func (t api) getInstallDisks(w http.ResponseWriter, r *http.Request) {
	disks, err := system.GetSystemDisks()
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

	if err := system.InstallToDisk(t.dbx, t.config, dbxState, req.Disk); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error installing to disk: "+err.Error())
		return
	}

	sendResponse(w, map[string]string{"status": "ok"})
}
