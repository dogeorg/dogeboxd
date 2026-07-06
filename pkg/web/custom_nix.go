package web

import (
	_ "embed"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
)

//go:embed custom.nix.default
var defaultCustomNix []byte

type GetCustomNixResponse struct {
	Content string `json:"content"`
	Exists  bool   `json:"exists"`
}

type SaveCustomNixRequest struct {
	Content string `json:"content"`
}

type ValidateCustomNixRequest struct {
	Content string `json:"content"`
}

type ValidateCustomNixResponse struct {
	Valid bool   `json:"valid"`
	Error string `json:"error,omitempty"`
}

func (t api) getCustomNix(w http.ResponseWriter, r *http.Request) {
	customNixPath := system.GetCustomNixPath(t.config)

	data, err := os.ReadFile(customNixPath)
	if err != nil {
		if os.IsNotExist(err) {
			legacyCustomNixPath := filepath.Join(t.config.NixDir, "custom.nix")
			legacyData, legacyErr := os.ReadFile(legacyCustomNixPath)
			if legacyErr == nil {
				sendResponse(w, GetCustomNixResponse{
					Content: string(legacyData),
					Exists:  true,
				})
				return
			}
			if os.IsNotExist(legacyErr) {
				// File doesn't exist in either location, return default template.
				sendResponse(w, GetCustomNixResponse{
					Content: string(defaultCustomNix),
					Exists:  false,
				})
				return
			}
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to read legacy custom.nix")
			return
		}
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to read custom.nix")
		return
	}

	sendResponse(w, GetCustomNixResponse{
		Content: string(data),
		Exists:  true,
	})
}

func (t api) saveCustomNix(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req SaveCustomNixRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	// Queue the save action
	action := dogeboxd.SaveCustomNix{Content: req.Content}
	id := t.dbx.AddAction(action)
	sendResponse(w, map[string]string{"id": id})
}

func (t api) validateCustomNix(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req ValidateCustomNixRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	// Validate the nix content directly (synchronous, no job needed)
	validationErr := t.dbx.SystemUpdater.ValidateNix(req.Content)

	if validationErr != nil {
		sendResponse(w, ValidateCustomNixResponse{
			Valid: false,
			Error: validationErr.Error(),
		})
		return
	}

	sendResponse(w, ValidateCustomNixResponse{
		Valid: true,
	})
}
