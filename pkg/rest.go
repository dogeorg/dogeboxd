package dogeboxd

import (
	"context"
	"encoding/gob"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/dogeorg/dogeboxd/pkg/pup"
	"github.com/gorilla/securecookie"
	"github.com/rs/cors"
)

const sessionExpiry = time.Hour

type Session struct {
	Token      string
	Expiration time.Time
	DKM_TOKEN  string
}

var sessions []Session

func getBearerToken(r *http.Request) (bool, string) {
	authHeader := r.Header.Get("authorization")

	if authHeader == "" {
		return false, ""
	}

	authPart := strings.Split(authHeader, " ")

	if len(authPart) != 2 {
		return false, ""
	}

	return true, authPart[1]
}

func getSession(r *http.Request) (Session, bool) {
	tokenOK, token := getBearerToken(r)
	if !tokenOK || token == "" {
		return Session{}, false
	}

	for i, session := range sessions {
		if session.Token == token {

			if time.Now().After(session.Expiration) {
				// Expired.
				sessions = append(sessions[:i], sessions[i+1:]...)
				return Session{}, false
			}

			return session, true
		}
	}

	return Session{}, false
}

func storeSession(session Session, config ServerConfig) {
	sessions = append(sessions, session)

	if config.DevMode {
		file, err := os.OpenFile(fmt.Sprintf("%s/dev-sessions.gob", config.DataDir), os.O_RDWR|os.O_CREATE, 0666)
		if err == nil {
			encoder := gob.NewEncoder(file)
			err = encoder.Encode(sessions)
			if err != nil {
				log.Printf("Failed to encode sessions to dev-sessions.gob: %v", err)
			}
			file.Close()
		} else {
			log.Printf("Failed to open dev-sessions.gob: %v, ignoring..", err)
		}
	}
}

func newSession() (string, Session) {
	tokenBytes := securecookie.GenerateRandomKey(32)
	tokenHex := make([]byte, hex.EncodedLen(len(tokenBytes)))
	hex.Encode(tokenHex, tokenBytes)
	token := string(tokenHex)
	session := Session{
		Token:      token,
		Expiration: time.Now().Add(sessionExpiry),
	}
	return token, session
}

func delSession(r *http.Request) error {
	tokenOK, token := getBearerToken(r)
	if !tokenOK || token == "" {
		return errors.New("failed to fetch bearer token")
	}

	for i, session := range sessions {
		if session.Token == token {
			sessions = append(sessions[:i], sessions[i+1:]...)
			return nil
		}
	}

	return nil
}

func authReq(dbx Dogeboxd, route string, next http.HandlerFunc) http.HandlerFunc {
	if route == "POST /authenticate" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			next.ServeHTTP(w, r)
		})
	}

	sessionHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, ok := getSession(r)

		if !ok {
			w.WriteHeader(401)
			return
		}

		next.ServeHTTP(w, r)
	})

	// We don't want a few routes to be locked down until the user has actually configured their system.
	// Whitelist those here.
	// TODO: Don't hardcode these.
	if route == "GET /system/bootstrap" ||
		route == "POST /system/bootstrap" ||
		route == "GET /system/network/list" ||
		route == "PUT /system/network/set-pending" ||
		route == "GET /keys" ||
		route == "POST /keys/create-master" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			dbxis := dbx.sm.Get().Dogebox.InitialState

			if !dbxis.HasFullyConfigured {
				// We good.
				next.ServeHTTP(w, r)
				return
			}

			// Still check.
			sessionHandler.ServeHTTP(w, r)
		})
	}

	// Any other function should require an authed session
	return sessionHandler
}

func RESTAPI(config ServerConfig, dbx Dogeboxd, pups PupManager, ws WSRelay, sources SourceManager) conductor.Service {
	sessions = []Session{}

	if config.DevMode {
		log.Println("In development mode: Loading REST API sessions..")
		file, err := os.Open(fmt.Sprintf("%s/dev-sessions.gob", config.DataDir))
		if err == nil {
			decoder := gob.NewDecoder(file)
			err = decoder.Decode(&sessions)
			if err != nil {
				log.Printf("Failed to decode sessions from dev-sessions.gob: %v", err)
			}
			file.Close()
			log.Printf("Loaded %d sessions from dev-sessions.gob", len(sessions))
		} else {
			log.Printf("Failed to open dev-sessions.gob: %v", err)
		}
	}

	dkm := NewDKMManager(pups)

	a := api{
		mux:     http.NewServeMux(),
		config:  config,
		dbx:     dbx,
		pups:    pups,
		ws:      ws,
		dkm:     dkm,
		sources: sources,
	}

	routes := map[string]http.HandlerFunc{}

	// Recovery routes are the _only_ routes loaded in recovery mode.
	recoveryRoutes := map[string]http.HandlerFunc{
		"POST /authenticate":              a.authenticate,
		"POST /logout":                    a.logout,
		"GET /system/bootstrap":           a.getBootstrap,
		"GET /system/network/list":        a.getNetwork,
		"PUT /system/network/set-pending": a.setPendingNetwork,
		"POST /system/network/connect":    a.connectNetwork,
		"POST /system/host/shutdown":      a.hostShutdown,
		"POST /system/host/reboot":        a.hostReboot,
		"POST /keys/create-master":        a.createMasterKey,
		"GET /keys":                       a.listKeys,
		"POST /system/bootstrap":          a.initialBootstrap,
	}

	// Normal routes are used when we are not in recovery mode.
	// nb. These are used in _addition_ to recovery routes.
	normalRoutes := map[string]http.HandlerFunc{
		"POST /pup/{ID}/{action}": a.pupAction,
		"PUT /pup":                a.installPup,
		"POST /config/{PupID}":    a.updateConfig,
		"GET /sources":            a.getSources,
		"PUT /source":             a.createSource,
		"GET /sources/store":      a.getStoreList,
		"DELETE /source/{name}":   a.deleteSource,
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
		a.mux.HandleFunc(p, authReq(dbx, p, h))
	}

	return a
}

type api struct {
	dbx     Dogeboxd
	dkm     DKMManager
	mux     *http.ServeMux
	pups    PupManager
	config  ServerConfig
	ws      WSRelay
	sources SourceManager
}

type CreateMasterKeyRequestBody struct {
	Password string `json:"password"`
}

type AuthenticateRequestBody struct {
	Password string `json:"password"`
}

type InitialSystemBootstrapRequestBody struct {
	ReflectorToken string `json:"reflectorToken"`
}

func (t api) authenticate(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody AuthenticateRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
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
		// Wrong password.
		sendErrorResponse(w, 403, "Invalid password")
		return
	}

	// We've authed. Save our dkm authentication token to a new session.
	token, session := newSession()
	session.DKM_TOKEN = dkmToken
	storeSession(session, t.config)

	sendResponse(w, map[string]any{
		"success": true,
		"token":   token,
	})
}

func (t api) logout(w http.ResponseWriter, r *http.Request) {
	session, sessionOK := getSession(r)
	if !sessionOK {
		sendErrorResponse(w, 500, "Failed to fetch session")
		return
	}

	// Clear our DKM token first. This ensures we can still convey an error
	// to the user if this fails for whatever reason. UI should tell them to
	// reboot their box or something to clear all authed sessions.
	ok, err := t.dkm.InvalidateToken(session.DKM_TOKEN)
	if err != nil {
		log.Println("failed to invalidate token with DKM:", err)
		sendErrorResponse(w, 500, err.Error())
		return
	}

	if !ok {
		log.Println("DKM returned ok=false when invalidating token")
		sendErrorResponse(w, 500, "Failed to invalidate token")
		return
	}

	delSession(r)

	sendResponse(w, map[string]any{
		"success": true,
	})
}

func (t api) getRawBS() (any, error) {
	dbxState := t.dbx.sm.Get().Dogebox

	list, err := t.sources.GetAll()
	if err != nil {
		return nil, err
	}

	flat := []pup.PupManifest{}

	for _, l := range list {
		for _, pup := range l.Pups {
			flat = append(flat, pup.Manifest)
		}
	}

	// TODO: Ideally this should return the straight map to the client,
	//       but the frontend is currently just expecting an array.

	return map[string]any{
		"manifests": flat,
		"states":    t.pups.GetStateMap(),
		"stats":     t.pups.GetStatsMap(),
		"setupFacts": map[string]bool{
			"hasGeneratedKey":                  dbxState.InitialState.HasGeneratedKey,
			"hasConfiguredNetwork":             dbxState.InitialState.HasSetNetwork,
			"hasCompletedInitialConfiguration": dbxState.InitialState.HasFullyConfigured,
		},
	}, nil
}

func (t api) getBootstrap(w http.ResponseWriter, r *http.Request) {
	bs, err := t.getRawBS()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error fetching bootstrap")
		return
	}

	sendResponse(w, bs)
}

func (t api) hostReboot(w http.ResponseWriter, r *http.Request) {
	t.dbx.lifecycle.Reboot()
}

func (t api) hostShutdown(w http.ResponseWriter, r *http.Request) {
	t.dbx.lifecycle.Shutdown()
}

func (t api) createMasterKey(w http.ResponseWriter, r *http.Request) {
	// If we've already created a master key, return an error.
	// Probably probably _also_ want to check with DKM that we don't have
	// a key created, but there's currently no API to do so.
	if t.dbx.sm.Get().Dogebox.InitialState.HasGeneratedKey {
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

	dbxs := t.dbx.sm.Get().Dogebox

	// TODO: this shouldn't live in here.
	if !dbxs.InitialState.HasGeneratedKey {
		dbxs.InitialState.HasGeneratedKey = true
		t.dbx.sm.SetDogebox(dbxs)
		if err := t.dbx.sm.Save(); err != nil {
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
	dbxis := t.dbx.sm.Get().Dogebox.InitialState

	keyResponse := []map[string]any{}

	if dbxis.HasGeneratedKey {
		keyResponse = append(keyResponse, map[string]any{"type": "master"})
	}

	sendResponse(w, map[string]any{
		"keys": keyResponse,
	})
}

func (t api) initialBootstrap(w http.ResponseWriter, r *http.Request) {
	// Check a few things first.
	if !t.config.Recovery {
		sendErrorResponse(w, http.StatusForbidden, "Cannot initiate bootstrap in non-recovery mode.")
		return
	}

	dbxis := t.dbx.sm.Get().Dogebox.InitialState

	if dbxis.HasFullyConfigured {
		sendErrorResponse(w, http.StatusForbidden, "System has already been initialised")
		return
	}

	if !dbxis.HasGeneratedKey || !dbxis.HasSetNetwork {
		sendErrorResponse(w, http.StatusForbidden, "System not ready to initialise")
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var requestBody InitialSystemBootstrapRequestBody
	if err := json.Unmarshal(body, &requestBody); err != nil {
		http.Error(w, "Error parsing payload", http.StatusBadRequest)
		return
	}

	// OK.

	// TODO: turn off AP
	// TODO: connect to network.
	// TODO: ensure network actually connects.

	if requestBody.ReflectorToken != "" {
		// TODO: ping reflector with relevant internal IP
	}

	dbxs := t.dbx.sm.Get().Dogebox
	dbxs.InitialState.HasFullyConfigured = true
	t.dbx.sm.SetDogebox(dbxs)

	if err := t.dbx.sm.Save(); err != nil {
		// What should we do here? We've already turned off AP mode so any errors
		// won't get send back to the client. I guess we just reboot?
		// That'll force recovery mode again. We can't even persist this error though.
		sendErrorResponse(w, http.StatusInternalServerError, "Error persisting flags")
	}

	sendResponse(w, map[string]any{"status": "OK"})
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

	dbxs := t.dbx.sm.Get().Dogebox

	// TODO: this shouldn't live in here.
	if !dbxs.InitialState.HasSetNetwork {
		dbxs.InitialState.HasSetNetwork = true
		t.dbx.sm.SetDogebox(dbxs)
		if err := t.dbx.sm.Save(); err != nil {
			sendErrorResponse(w, http.StatusInternalServerError, "Failed to persist network set flag")
			return
		}
	}

	id := t.dbx.AddAction(UpdatePendingSystemNetwork{Network: selectedNetwork})
	sendResponse(w, map[string]string{"id": id})
}

func (t api) getSources(w http.ResponseWriter, r *http.Request) {
	sources := t.sources.GetAllSourceConfigurations()

	sendResponse(w, map[string]any{
		"success": true,
		"sources": sources,
	})
}

func (t api) createSource(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		sendErrorResponse(w, http.StatusBadRequest, "Error reading request body")
		return
	}
	defer r.Body.Close()

	var req ManifestSourceConfiguration
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

type StoreListSourceEntryPup struct {
	IsInstalled      bool                       `json:"isInstalled"`
	InstalledVersion string                     `json:"installedVersion"`
	Versions         map[string]pup.PupManifest `json:"versions"`
}

type StoreListSourceEntry struct {
	LastUpdated string                             `json:"lastUpdated"`
	Pups        map[string]StoreListSourceEntryPup `json:"pups"`
}

func (t api) getStoreList(w http.ResponseWriter, r *http.Request) {
	available, err := t.sources.GetAll()
	if err != nil {
		sendErrorResponse(w, http.StatusInternalServerError, "Error fetching sources")
		return
	}

	response := map[string]StoreListSourceEntry{}

	for k, source := range available {
		pups := map[string]StoreListSourceEntryPup{}

		for _, availablePup := range source.Pups {
			// Check if we already have a pup in our list for this version.
			if _, ok := pups[availablePup.Name]; !ok {
				// TODO: Ideally not have to do this lookup.
				s, err := t.sources.GetSource(k)
				if err != nil {
					sendErrorResponse(w, http.StatusInternalServerError, "Error fetching source")
					return
				}

				// Check in our pup manager to see if this pup is installed.
				// If it is, we set the InstalledVersion.
				installedPupState := t.dbx.Pups.GetPupFromSource(availablePup.Name, s.Config())

				isInstalled := installedPupState != nil

				var installedVersion string

				if isInstalled {
					installedVersion = installedPupState.Version
				}

				versions := map[string]pup.PupManifest{}

				pups[availablePup.Name] = StoreListSourceEntryPup{
					IsInstalled:      isInstalled,
					InstalledVersion: installedVersion,
					Versions:         versions,
				}
			}

			// Retrieve the struct, modify it, and store it back in the map
			pupEntry := pups[availablePup.Name]
			pupEntry.Versions[availablePup.Version] = availablePup.Manifest
			pups[availablePup.Name] = pupEntry
		}

		response[k] = StoreListSourceEntry{
			LastUpdated: source.LastUpdated.Format(time.RFC3339),
			Pups:        pups,
		}
	}

	sendResponse(w, response)
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
		bs, err := t.getRawBS()
		if err != nil {
			return Change{ID: "internal", Error: "Failed to fetch bootstrap"}
		}

		return Change{ID: "internal", Error: "", Type: "bootstrap", Update: bs}
	}).ServeHTTP(w, r)
}

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
	id := t.dbx.AddAction(UpdatePupConfig{PupID: pupid, Payload: data})
	sendResponse(w, map[string]string{"id": id})
}

type InstallPupRequest struct {
	PupName    string `json:"pupName"`
	PupVersion string `json:"pupVersion"`
	SourceName string `json:"sourceName"`
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

	id := t.dbx.AddAction(InstallPup(req))
	sendResponse(w, map[string]string{"id": id})
}

func (t api) pupAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("ID")
	action := r.PathValue("action")

	if action == "install" {
		sendErrorResponse(w, http.StatusBadRequest, "Must use PUT /pup to install")
		return
	}

	var a Action
	switch action {
	case "uninstall":
		a = UninstallPup{PupID: id}
	case "purge":
		a = PurgePup{PupID: id}
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
