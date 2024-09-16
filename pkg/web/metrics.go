package web

import (
	"encoding/json"
	"io"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t InternalRouter) recordMetrics(w http.ResponseWriter, r *http.Request) {
	var originIsPup bool = false
	originIP := getOriginIP(r)
	originPup, _, err := t.pm.FindPupByIP(originIP)
	if err == nil {
		originIsPup = true
	}

	if !originIsPup {
		// you must be a pup!
		forbidden(w, "You are not a Pup we know about", originIP)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	data := make(map[string]dogeboxd.PupMetric)
	err = json.Unmarshal(body, &data)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error unmarshalling JSON")
		return
	}

	update := dogeboxd.UpdateMetrics{
		PupID:   originPup.ID,
		Payload: data,
	}

	id := t.dbx.AddAction(update)
	sendResponse(w, map[string]string{"id": id})
}
