package web

import (
	"encoding/json"
	"io"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type SetSSHStateRequest struct {
	Enabled string `json:"enabled"`
}

type AddSSHKeyRequest struct {
	Key string `json:"key"`
}

func (t api) setSSHState(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req SetSSHStateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	var action dogeboxd.Action
	if req.Enabled == "true" {
		action = dogeboxd.EnableSSH{}
	} else {
		action = dogeboxd.DisableSSH{}
	}

	id := t.dbx.AddAction(action)
	sendResponse(w, map[string]string{"id": id})
}

func (t api) listSSHKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := t.dbx.SystemUpdater.ListSSHKeys()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error listing SSH keys")
		return
	}

	sendResponse(w, map[string]any{"keys": keys})
}

func (t api) addSSHKey(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req AddSSHKeyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	if req.Key == "" {
		sendErrorResponse(w, http.StatusBadRequest, "SSH key is required")
		return
	}

	id := t.dbx.AddAction(dogeboxd.AddSSHKey{Key: req.Key})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) removeSSHKey(w http.ResponseWriter, r *http.Request) {
	keyId := r.PathValue("id")
	if keyId == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Key ID is required")
		return
	}

	id := t.dbx.AddAction(dogeboxd.RemoveSSHKey{ID: keyId})
	sendResponse(w, map[string]string{"id": id})
}
