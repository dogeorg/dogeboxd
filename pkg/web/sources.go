package web

import (
	"encoding/json"
	"io"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t api) createSource(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var req dogeboxd.ManifestSourceConfiguration
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error parsing payload")
		return
	}

	if _, err := t.sources.AddSource(req); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error adding source")
		return
	}

	sendResponse(w, map[string]any{
		"success": true,
	})
}

func (t api) deleteSource(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	if name == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Missing source name")
		return
	}

	if err := t.sources.RemoveSource(name); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error deleting source")
		return
	}

	sendResponse(w, map[string]any{
		"success": true,
	})
}
