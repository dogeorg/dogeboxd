package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t api) getPupMetrics(w http.ResponseWriter, r *http.Request) {
	pupID := r.PathValue("ID")
	lastOnly := r.URL.Query().Get("last") == "true"

	metrics := t.dbx.Pups.GetMetrics(pupID)

	if !lastOnly {
		sendResponse(w, metrics)
		return
	}

	lastMetrics := make(map[string]interface{})
	for name, buffer := range metrics {
		switch bufferSlice := buffer.(type) {
		case []string:
			if len(bufferSlice) > 0 {
				lastMetrics[name] = bufferSlice[0]
			}
		case []int:
			if len(bufferSlice) > 0 {
				lastMetrics[name] = bufferSlice[0]
			}
		case []float64:
			if len(bufferSlice) > 0 {
				lastMetrics[name] = bufferSlice[0]
			}
		default:
			log.Printf("Unexpected buffer type for metric %s", name)
		}
	}

	sendResponse(w, lastMetrics)
}

func (t InternalRouter) recordMetrics(w http.ResponseWriter, r *http.Request) {
	originPup, ok := t.getOriginPup(r)
	if !ok {
		// you must be a pup!
		forbidden(w, "You are not a Pup we know about")
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
