package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t api) updateConfig(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	data := make(map[string]string)
	err = json.Unmarshal(body, &data)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}
	id := t.dbx.AddAction(dogeboxd.UpdatePupConfig{PupID: pupid, Payload: data})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) updateProviders(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	data := make(map[string]string)
	err = json.Unmarshal(body, &data)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}
	id := t.dbx.AddAction(dogeboxd.UpdatePupProviders{PupID: pupid, Payload: data})
	sendResponse(w, map[string]string{"id": id})
}

type InstallPupRequest struct {
	PupName    string `json:"pupName"`
	PupVersion string `json:"pupVersion"`
	SourceId   string `json:"sourceId"`
}

func (t api) installPup(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var req InstallPupRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	id := t.dbx.AddAction(dogeboxd.InstallPup(req))
	sendResponse(w, map[string]string{"id": id})
}

func (t api) pupAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("ID")
	action := r.PathValue("action")

	if action == "install" {
		sendErrorResponse(w, http.StatusBadRequest, "Must use PUT /pup to install")
		return
	}

	var a dogeboxd.Action
	switch action {
	case "uninstall":
		a = dogeboxd.UninstallPup{PupID: id}
	case "purge":
		a = dogeboxd.PurgePup{PupID: id}
	case "enable":
		a = dogeboxd.EnablePup{PupID: id}
	case "disable":
		a = dogeboxd.DisablePup{PupID: id}
	default:
		sendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("No pup action %s", action))
		return
	}

	sendResponse(w, map[string]string{"id": t.dbx.AddAction(a)})
}
