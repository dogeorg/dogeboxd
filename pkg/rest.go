package dogeboxd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"

	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/rs/cors"
)

func RESTAPI(config ServerConfig, dbx Dogeboxd, man ManifestIndex, pups PupManager, ws WSRelay) conductor.Service {
	a := api{mux: http.NewServeMux(), config: config, dbx: dbx, man: man, pups: pups, ws: ws}

	routes := map[string]http.HandlerFunc{}

	// Recovery routes are the _only_ routes loaded in recovery mode.
	recoveryRoutes := map[string]http.HandlerFunc{
		"GET /bootstrap/":                 a.getBootstrap,
		"GET /system/network/list":        a.getNetwork,
		"PUT /system/network/set-pending": a.setPendingNetwork,
		"POST /system/network/connect":    a.connectNetwork,
		"POST /system/host/shutdown":      a.hostShutdown,
		"POST /system/host/reboot":        a.hostReboot,
	}

	// Normal routes are used when we are not in recovery mode.
	// nb. These are used in _addition_ to recovery routes.
	normalRoutes := map[string]http.HandlerFunc{
		"POST /pup/{ID}/{action}": a.pupAction,
		"POST /config/{PupID}":    a.updateConfig,
		"/ws/log/{PupID}":         a.getLogSocket,
		"/ws/state/":              a.getUpdateSocket,
	}

	// We always want to load recovery routes.
	for k, v := range recoveryRoutes {
		routes[k] = v
	}

	// If we're not in recovery mode, also load our normal routes.
	if !config.Recovery {
		for k, v := range normalRoutes {
			routes[k] = v
		}
		log.Printf("Loaded %d API routes", len(routes))
	} else {
		log.Printf("In recovery mode: Loading limited routes")
	}

	for p, h := range routes {
		a.mux.HandleFunc(p, h)
	}

	return a
}

type api struct {
	dbx    Dogeboxd
	mux    *http.ServeMux
	man    ManifestIndex
	pups   PupManager
	config ServerConfig
	ws     WSRelay
}

func (t api) getRawBS() any {
	return map[string]any{
		"manifests": t.man.GetManifestMap(),
		"states":    t.pups.GetStateMap(),
		"stats":     t.pups.GetStatsMap(),
	}
}

func (t api) getBootstrap(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, t.getRawBS())
}

func (t api) hostReboot(w http.ResponseWriter, r *http.Request) {
	t.dbx.lifecycle.Reboot()
}

func (t api) hostShutdown(w http.ResponseWriter, r *http.Request) {
	t.dbx.lifecycle.Shutdown()
}

func (t api) getNetwork(w http.ResponseWriter, r *http.Request) {
	sendResponse(w, map[string]any{
		"success":  true,
		"networks": t.dbx.NetworkManager.GetAvailableNetworks(),
	})
}

func (t api) connectNetwork(w http.ResponseWriter, r *http.Request) {
	err := t.dbx.NetworkManager.TryConnect()
	// Chances are we'll never actually get here, because you'll probably be disconnected
	// from the box once (if) it changes networks, and your connection will break.
	if err != nil {
		log.Printf("Failed to connect to network: %+v", err)
		sendErrorResponse(w, http.StatusInternalServerError, "Failed to connect to network")
		return
	}

	sendResponse(w, map[string]bool{"success": true})
}

func (t api) setPendingNetwork(w http.ResponseWriter, r *http.Request) {
	body, err := ioutil.ReadAll(r.Body)
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

	var selectedNetwork SelectedNetwork

	// We need proper input validation here.
	if _, ok := rawNetwork["interface"]; ok {
		if _, ok := rawNetwork["ssid"]; ok {
			var wifiNetwork SelectedNetworkWifi
			if err := json.Unmarshal(body, &wifiNetwork); err != nil {
				http.Error(w, "Error parsing WiFi network JSON", http.StatusBadRequest)
				return
			}
			selectedNetwork = wifiNetwork
		} else {
			var ethernetNetwork SelectedNetworkEthernet
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

	id := t.dbx.AddAction(UpdatePendingSystemNetwork{Network: selectedNetwork})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) getLogSocket(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	cancel, logChan, err := t.dbx.GetLogChannel(pupid)
	if err != nil {
		fmt.Println("ERR", err)
		sendErrorResponse(w, http.StatusBadRequest, "Error establishing log channel")
	}
	t.ws.GetWSChannelHandler(fmt.Sprintf("%s-log", pupid), logChan, cancel).ServeHTTP(w, r)
}

func (t api) getUpdateSocket(w http.ResponseWriter, r *http.Request) {
	t.ws.GetWSHandler(WS_DEFAULT_CHANNEL, func() any {
		return Change{ID: "internal", Error: "", Type: "bootstrap", Update: t.getRawBS()}
	}).ServeHTTP(w, r)
}

func (t api) updateConfig(w http.ResponseWriter, r *http.Request) {
	pupid := r.PathValue("PupID")
	body, err := ioutil.ReadAll(r.Body)
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
	id := t.dbx.AddAction(UpdatePupConfig{PupID: pupid, Payload: data})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) pupAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("ID")
	action := r.PathValue("action")
	var a Action
	switch action {
	case "install":
		a = InstallPup{ManifestID: id}
	case "uninstall":
		a = UninstallPup{PupID: id}
	case "enable":
		a = EnablePup{PupID: id}
	case "disable":
		a = DisablePup{PupID: id}
	default:
		sendErrorResponse(w, http.StatusNotFound, fmt.Sprintf("No pup action %s", action))
		return
	}

	sendResponse(w, map[string]string{"id": t.dbx.AddAction(a)})
}

func (t api) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		handler := cors.AllowAll().Handler(t.mux)
		srv := &http.Server{Addr: fmt.Sprintf("%s:%d", t.config.Bind, t.config.Port), Handler: handler}
		go func() {
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP server public ListenAndServe: %v", err)
			}
		}()

		started <- true
		ctx := <-stop
		srv.Shutdown(ctx)
		stopped <- true
	}()
	return nil
}

// Helpers
func sendResponse(w http.ResponseWriter, payload any) {
	// note: w.Header after this, so we can call sendError
	b, err := json.Marshal(payload)
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, fmt.Sprintf("in json.Marshal: %s", err.Error()))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // do not cache (Browsers cache GET forever by default)
	w.Write(b)
}

func sendErrorResponse(w http.ResponseWriter, code int, message string) {
	log.Printf("[!] %d: %s\n", code, message)
	// would prefer to use json.Marshal, but this avoids the need
	// to handle encoding errors arising from json.Marshal itself!
	payload := fmt.Sprintf("{\"error\":{\"code\":%q,\"message\":%q}}", code, message)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store") // do not cache (Browsers cache GET forever by default)
	w.WriteHeader(code)
	w.Write([]byte(payload))
}
