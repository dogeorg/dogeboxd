package web

import (
	"encoding/json"
	"io"
	"net/http"
)

type CreateSourceRequest struct {
	Location string `json:"location"`
}

func (t api) createSource(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var req CreateSourceRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error parsing payload")
		return
	}

	if _, err := t.sources.AddSource(req.Location); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error adding source")
		return
	}

	sendResponse(w, map[string]any{
		"success": true,
	})
}

func (t api) deleteSource(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	if id == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Missing source id")
		return
	}

	if err := t.sources.RemoveSource(id); err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error deleting source")
		return
	}

	sendResponse(w, map[string]any{
		"success": true,
	})
}
