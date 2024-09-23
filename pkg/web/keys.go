package web

import (
	"encoding/json"
	"io"
	"net/http"
)

type CreateMasterKeyRequestBody struct {
	Password string `json:"password"`
}

func (t api) createMasterKey(w http.ResponseWriter, r *http.Request) {
	// If we've already created a master key, return an error.
	// Probably probably _also_ want to check with DKM that we don't have
	// a key created, but there's currently no API to do so.
	if t.sm.Get().Dogebox.InitialState.HasGeneratedKey {
		sendErrorResponse(w, http.StatusForbidden, "A master key already exists")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody CreateMasterKeyRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	// Assuming the password send is anything but an empty string, we allow it.
	// We don't want to add any restrictions, rather the frontend UI will have suggestions.
	if requestBody.Password == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Password cannot be empty")
		return
	}

	seedPhrase, err := t.dkm.CreateKey(requestBody.Password)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to create key in DKM")
		return
	}

	dbxs := t.sm.Get().Dogebox

	// TODO: this shouldn't live in here.
	if !dbxs.InitialState.HasGeneratedKey {
		dbxs.InitialState.HasGeneratedKey = true
		t.sm.SetDogebox(dbxs)
		if err := t.sm.Save(); err != nil {
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to persist key generation flag")
			return
		}
	}

	dkmToken, dkmError, err := t.dkm.Authenticate(requestBody.Password)
	if err != nil {
		sendErrorResponse(w, 500, err.Error())
		return
	}

	if dkmError != nil {
		sendErrorResponse(w, 403, dkmError.Error())
		return
	}

	if dkmToken == "" {
		// We should never get here, seeing as we are using
		// the same password as we just encrypted our key with..
		sendErrorResponse(w, 403, "Invalid password")
		return
	}

	// We've authed. Save our dkm authentication token to a new session.
	token, session := newSession()
	session.DKM_TOKEN = dkmToken
	storeSession(session, t.config)

	sendResponse(w, map[string]any{
		"success":    true,
		"seedPhrase": seedPhrase,
		"token":      token,
	})
}

// The frontend requires this endpoint, but we should remove.
func (t api) listKeys(w http.ResponseWriter, r *http.Request) {
	dbxis := t.sm.Get().Dogebox.InitialState

	keyResponse := []map[string]any{}

	if dbxis.HasGeneratedKey {
		keyResponse = append(keyResponse, map[string]any{"type": "master"})
	}

	sendResponse(w, map[string]any{
		"keys": keyResponse,
	})
}

// Delegate keys to pups based on their pupID
func (t InternalRouter) getDelegatedPupKeys(w http.ResponseWriter, r *http.Request) {
	originPup, ok := t.getOriginPup(r)
	if !ok {
		// you must be a pup!
		forbidden(w, "You are not a Pup we know about")
		return
	}
	sendResponse(w, map[string]any{
		"keys": originPup.ID, // TODO
	})
}
