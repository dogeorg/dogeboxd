package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func (t api) getNetwork(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, map[string]any{
		"success":  true,
		"networks": t.dbx.NetworkManager.GetAvailableNetworks(),
	})
}

func (t api) connectNetwork(w http.ResponseWriter, r *http.Request) {
	nixPatch := t.nix.NewPatch(dogeboxd.NewConsoleSubLogger("internal", "set network"))

	err := t.dbx.NetworkManager.TryConnect(nixPatch)
	// Chances are we'll never actually get here, because you'll probably be disconnected
	// from the box once (if) it changes networks, and your connection will break.
	if err != nil {
		log.Printf("Failed to connect to network: %+v", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to connect to network")
		return
	}

	if err := nixPatch.Apply(); err != nil {
		log.Printf("Failed to apply nix patch: %+v", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to apply nix patch")
		return
	}

	sendResponse(w, map[string]bool{"success": true})
}

func (t api) setPendingNetwork(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	// Unmarshal the JSON into a map first to determine the network type
	var rawNetwork map[string]interface{}
	if err := json.Unmarshal(body, &rawNetwork); err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error parsing JSON")
		return
	}

	var selectedNetwork dogeboxd.SelectedNetwork

	// We need proper input validation here.
	if _, ok := rawNetwork["interface"]; ok {
		if _, ok := rawNetwork["ssid"]; ok {
			var wifiNetwork dogeboxd.SelectedNetworkWifi
			if err := json.Unmarshal(body, &wifiNetwork); err != nil {
				http.Error(w, "Error parsing WiFi network JSON", http.StatusBadRequest)
				return
			}
			selectedNetwork = wifiNetwork
		} else {
			var ethernetNetwork dogeboxd.SelectedNetworkEthernet
			if err := json.Unmarshal(body, &ethernetNetwork); err != nil {
				http.Error(w, "Error parsing Ethernet network JSON", http.StatusBadRequest)
				return
			}
			selectedNetwork = ethernetNetwork
		}
	} else {
		http.Error(w, "Invalid network type", http.StatusBadRequest)
		return
	}

	dbxs := t.sm.Get().Dogebox

	// TODO: this shouldn't live in here.
	if !dbxs.InitialState.HasSetNetwork {
		dbxs.InitialState.HasSetNetwork = true
		if err := t.sm.SetDogebox(dbxs); err != nil {
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to persist network set flag")
			return
		}
	}

	id := t.dbx.AddAction(dogeboxd.UpdatePendingSystemNetwork{Network: selectedNetwork})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) getSources(w http.ResponseWriter, r *http.Request) {
	sources := t.sources.GetAllSourceConfigurations()

	sendResponse(w, map[string]any{
		"success": true,
		"sources": sources,
	})
}
