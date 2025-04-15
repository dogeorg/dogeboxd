package web

import (
	"context"
	"encoding/gob"
	"fmt"
	"log"
	"net/http"
	"os"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/rs/cors"
)

func RESTAPI(
	config dogeboxd.ServerConfig,
	sm dogeboxd.StateManager,
	dbx dogeboxd.Dogeboxd,
	pups dogeboxd.PupManager,
	sources dogeboxd.SourceManager,
	lifecycle dogeboxd.LifecycleManager,
	nix dogeboxd.NixManager,
	dkm dogeboxd.DKMManager,
	ws WSRelay,
) conductor.Service {
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

	a := api{
		mux:       http.NewServeMux(),
		config:    config,
		sm:        sm,
		dbx:       dbx,
		pups:      pups,
		ws:        ws,
		dkm:       dkm,
		lifecycle: lifecycle,
		nix:       nix,
		sources:   sources,
	}

	routes := map[string]http.HandlerFunc{}

	// Recovery routes are the _only_ routes loaded in recovery mode.
	recoveryRoutes := map[string]http.HandlerFunc{
		"POST /authenticate": a.authenticate,
		"POST /logout":       a.logout,

		"GET /system/bootstrap":          a.getBootstrap,
		"GET /system/recovery-bootstrap": a.getRecoveryBootstrap,
		"GET /system/keymaps":            a.getKeymaps,
		"POST /system/keymap":            a.setKeyMap,
		"GET /system/disks":              a.getInstallDisks,
		"POST /system/hostname":          a.setHostname,
		"POST /system/storage":           a.setStorageDevice,
		"POST /system/install":           a.installToDisk,

		"GET /system/network/list":        a.getNetwork,
		"PUT /system/network/set-pending": a.setPendingNetwork,
		"POST /system/network/connect":    a.connectNetwork,
		"POST /system/host/shutdown":      a.hostShutdown,
		"POST /system/host/reboot":        a.hostReboot,
		"POST /keys/create-master":        a.createMasterKey,
		"GET /keys":                       a.listKeys,
		"POST /system/bootstrap":          a.initialBootstrap,

		"GET /system/ssh/state":       a.getSSHState,
		"PUT /system/ssh/state":       a.setSSHState,
		"GET /system/ssh/keys":        a.listSSHKeys,
		"PUT /system/ssh/key":         a.addSSHKey,
		"DELETE /system/ssh/key/{id}": a.removeSSHKey,
		"/ws/state/":                  a.getUpdateSocket,
	}

	// Normal routes are used when we are not in recovery mode.
	// nb. These are used in _addition_ to recovery routes.
	normalRoutes := map[string]http.HandlerFunc{
		"GET /pup/{ID}/metrics":   a.getPupMetrics,
		"POST /pup/{ID}/{action}": a.pupAction,
		"PUT /pup":                a.installPup,
		"POST /config/{PupID}":    a.updateConfig,
		"POST /providers/{PupID}": a.updateProviders,
		"GET /providers/{PupID}":  a.getPupProviders,
		"POST /hooks/{PupID}":     a.updateHooks,
		"GET /sources":            a.getSources,
		"PUT /source":             a.createSource,
		"GET /sources/store":      a.getStoreList,
		"DELETE /source/{id}":     a.deleteSource,
		"/ws/log/{PupID}":         a.getLogSocket,
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
		a.mux.HandleFunc(p, authReq(dbx, sm, p, h))
	}

	return a
}

type api struct {
	dbx       dogeboxd.Dogeboxd
	sm        dogeboxd.StateManager
	dkm       dogeboxd.DKMManager
	mux       *http.ServeMux
	pups      dogeboxd.PupManager
	config    dogeboxd.ServerConfig
	sources   dogeboxd.SourceManager
	lifecycle dogeboxd.LifecycleManager
	nix       dogeboxd.NixManager
	ws        WSRelay
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
