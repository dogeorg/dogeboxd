package web

import (
	"encoding/json"
	"io"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

type AddBinaryCacheRequest struct {
	Host string `json:"host"`
	Key  string `json:"key"`
}

func (a api) getBinaryCaches(w http.ResponseWriter, r *http.Request) {
	dbxState := a.sm.Get().Dogebox
	sendResponse(w, dbxState.BinaryCaches)
}

func (a api) addBinaryCache(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}

	var req AddBinaryCacheRequest
	if err := json.Unmarshal(body, &req); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	dbxState := a.sm.Get().Dogebox

	for _, existingCache := range dbxState.BinaryCaches {
		if existingCache.Host == req.Host {
			sendErrorResponse(w, http.StatusBadRequest, "Binary cache with this host already exists")
			return
		}
		if existingCache.Key == req.Key {
			sendErrorResponse(w, http.StatusBadRequest, "Binary cache with this key already exists")
			return
		}
	}

	id := a.dbx.AddAction(dogeboxd.AddBinaryCache{Host: req.Host, Key: req.Key})
	sendResponse(w, map[string]string{"id": id})
}

func (a api) removeBinaryCache(w http.ResponseWriter, r *http.Request) {
	dbxState := a.sm.Get().Dogebox

	cacheId := r.PathValue("id")
	if cacheId == "" {
		sendErrorResponse(w, http.StatusBadRequest, "Cache ID is required")
		return
	}

	cacheFound := false
	for _, cache := range dbxState.BinaryCaches {
		if cache.ID == cacheId {
			cacheFound = true
			break
		}
	}

	if !cacheFound {
		sendErrorResponse(w, http.StatusBadRequest, "Binary cache with this ID does not exist")
		return
	}

	id := a.dbx.AddAction(dogeboxd.RemoveBinaryCache{ID: cacheId})
	sendResponse(w, map[string]string{"id": id})
}
