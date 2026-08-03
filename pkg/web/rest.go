package web

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/conductor"
	"github.com/Dogebox-WG/dogeboxd/protocol/gen/authenticate/v1/authenticatev1connect"
	"github.com/rs/cors"
)

type AuthState int

const (
	RequireAuth AuthState = iota
	ConfiguredAuth
	NoAuth
)

type RestHandlerFunc struct {
	auth_state AuthState
	handler    http.HandlerFunc
}

func rhf(a AuthState, h http.HandlerFunc) RestHandlerFunc {
	return RestHandlerFunc{auth_state: a, handler: h}
}

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
	sessions = newSessionManager(config, dkm)

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

	routes := map[string]RestHandlerFunc{}

	authenticator := &AuthenticateServer{a}
	authenticatePath, authenticateHandler := authenticatev1connect.NewAuthenticateServiceHandler(authenticator, connect.WithInterceptors(validate.NewInterceptor()))
	// Recovery routes are the _only_ routes loaded in recovery mode.
	recoveryRoutes := map[string]RestHandlerFunc{
		authenticatePath:        rhf(NoAuth, authenticateHandler.ServeHTTP),
		"POST /logout":          rhf(RequireAuth, a.logout),
		"POST /change-password": rhf(RequireAuth, a.changePassword),

		"GET /system/bootstrap":          rhf(ConfiguredAuth, a.getBootstrap),
		"GET /system/recovery-bootstrap": rhf(ConfiguredAuth, a.getRecoveryBootstrap),
		"GET /system/keymap":             rhf(RequireAuth, a.getKeymap),
		"GET /system/keymaps":            rhf(ConfiguredAuth, a.getKeymaps),
		"POST /system/keymap":            rhf(ConfiguredAuth, a.setKeyMap),
		"GET /system/timezone":           rhf(RequireAuth, a.getTimezone),
		"GET /system/timezones":          rhf(ConfiguredAuth, a.getTimezones),
		"POST /system/timezone":          rhf(ConfiguredAuth, a.setTimezone),
		"GET /system/disks":              rhf(ConfiguredAuth, a.getSystemDisks),
		"GET /system/install-disks":      rhf(ConfiguredAuth, a.getInstallDisks),
		"POST /system/hostname":          rhf(ConfiguredAuth, a.setHostname),
		"POST /system/storage":           rhf(ConfiguredAuth, a.setStorageDevice),
		"POST /system/install":           rhf(ConfiguredAuth, a.installToDisk),

		"GET /system/network/list":        rhf(ConfiguredAuth, a.getNetwork),
		"PUT /system/network/set-pending": rhf(ConfiguredAuth, a.setPendingNetwork),
		"POST /system/network/test":       rhf(ConfiguredAuth, a.testConnectNetwork),
		"POST /system/network/connect":    rhf(ConfiguredAuth, a.connectNetwork),
		"POST /system/host/shutdown":      rhf(ConfiguredAuth, a.hostShutdown),
		"POST /system/host/reboot":        rhf(ConfiguredAuth, a.hostReboot),
		"POST /keys/create-master":        rhf(ConfiguredAuth, a.createMasterKey),
		"GET /keys":                       rhf(ConfiguredAuth, a.listKeys),
		"POST /system/bootstrap":          rhf(ConfiguredAuth, a.initialBootstrap),

		"GET /system/ssh/state":               rhf(RequireAuth, a.getSSHState),
		"PUT /system/ssh/state":               rhf(RequireAuth, a.setSSHState),
		"GET /system/ssh/keys":                rhf(RequireAuth, a.listSSHKeys),
		"PUT /system/ssh/key":                 rhf(RequireAuth, a.addSSHKey),
		"DELETE /system/ssh/key/{id}":         rhf(RequireAuth, a.removeSSHKey),
		"GET /system/custom-nix":              rhf(RequireAuth, a.getCustomNix),
		"PUT /system/custom-nix":              rhf(RequireAuth, a.saveCustomNix),
		"POST /system/custom-nix/validate":    rhf(RequireAuth, a.validateCustomNix),
		"POST /system/import-blockchain-data": rhf(RequireAuth, a.importBlockchainData),
		"/ws/state/":                          rhf(ConfiguredAuth, a.getUpdateSocket),
		"/ws/jobs":                            rhf(RequireAuth, a.getJobsSocket),
		"/ws/log/job/{JobID}":                 rhf(RequireAuth, a.getJobLogSocket),
	}

	// Normal routes are used when we are not in recovery mode.
	// nb. These are used in _addition_ to recovery routes.
	normalRoutes := map[string]RestHandlerFunc{
		"GET /pup/{ID}/metrics":               rhf(RequireAuth, a.getPupMetrics),
		"POST /pup/{ID}/{action}":             rhf(RequireAuth, a.pupAction),
		"PUT /pup":                            rhf(RequireAuth, a.installPup),
		"PUT /pups":                           rhf(RequireAuth, a.installPups),
		"POST /config/{PupID}":                rhf(RequireAuth, a.updateConfig),
		"POST /providers/{PupID}":             rhf(RequireAuth, a.updateProviders),
		"GET /providers/{PupID}":              rhf(RequireAuth, a.getPupProviders),
		"POST /hooks/{PupID}":                 rhf(RequireAuth, a.updateHooks),
		"GET /sources":                        rhf(RequireAuth, a.getSources),
		"PUT /source":                         rhf(RequireAuth, a.createSource),
		"GET /sources/store":                  rhf(RequireAuth, a.getStoreList),
		"DELETE /source/{id}":                 rhf(RequireAuth, a.deleteSource),
		"GET /log/pup/{PupID}/download":       rhf(RequireAuth, a.downloadPupLog),
		"GET /log/job/{JobID}/download":       rhf(RequireAuth, a.downloadJobLog),
		"GET /log/pup/{PupID}/tail":           rhf(RequireAuth, a.getPupLogTail),
		"GET /log/job/{JobID}/tail":           rhf(RequireAuth, a.getJobLogTail),
		"/ws/log/pup/{PupID}":                 rhf(RequireAuth, a.getPupLogSocket),
		"/ws/log/job/{JobID}":                 rhf(RequireAuth, a.getJobLogSocket),
		"POST /system/welcome-complete":       rhf(RequireAuth, a.setWelcomeComplete),
		"POST /system/install-pup-collection": rhf(RequireAuth, a.installPupCollection),
		"GET /missing-deps/{PupID}":           rhf(RequireAuth, a.getMissingDeps),

		// Sidebar preferences
		"GET /system/sidebar-preferences":              rhf(RequireAuth, a.getSidebarPreferences),
		"POST /system/sidebar-preferences/pups/add":    rhf(RequireAuth, a.addSidebarPup),
		"POST /system/sidebar-preferences/pups/remove": rhf(RequireAuth, a.removeSidebarPup),

		"GET /system/binary-caches":        rhf(RequireAuth, a.getBinaryCaches),
		"PUT /system/binary-cache":         rhf(RequireAuth, a.addBinaryCache),
		"DELETE /system/binary-cache/{id}": rhf(RequireAuth, a.removeBinaryCache),

		// Pup update routes
		"GET /pup/updates":                    rhf(RequireAuth, a.getAllPupUpdates),
		"GET /pup/{pupId}/updates":            rhf(RequireAuth, a.getPupUpdates),
		"POST /pup/{pupId}/check-pup-updates": rhf(RequireAuth, a.checkPupUpdates),
		"POST /pup/{pupId}/upgrade":           rhf(RequireAuth, a.upgradePup),
		"POST /pup/{pupId}/update":            rhf(RequireAuth, a.updatePup), // Legacy, redirects to upgrade
		"POST /pup/{pupId}/rollback":          rhf(RequireAuth, a.rollbackPup),
		"GET /pup/{pupId}/previous-version":   rhf(RequireAuth, a.getPreviousVersion),
		"GET /pup/skipped-updates":            rhf(RequireAuth, a.getAllSkippedUpdates),
		"POST /pup/{pupId}/skip-update":       rhf(RequireAuth, a.skipPupUpdate),
		"DELETE /pup/{pupId}/skip-update":     rhf(RequireAuth, a.clearSkippedUpdate),

		"GET /system/updates": rhf(RequireAuth, a.checkForUpdates),
		"POST /system/update": rhf(RequireAuth, a.commenceUpdate),

		"GET /system/stats":    rhf(RequireAuth, a.getSystemStats),
		"GET /system/services": rhf(RequireAuth, a.getSystemServices),

		// Job management routes
		"GET /jobs":                              rhf(RequireAuth, a.getJobs),
		"GET /jobs/active":                       rhf(RequireAuth, a.getActiveJobs),
		"GET /jobs/recent":                       rhf(RequireAuth, a.getRecentJobs),
		"GET /jobs/stats":                        rhf(RequireAuth, a.getJobStats),
		"GET /jobs/{jobID}":                      rhf(RequireAuth, a.getJob),
		"DELETE /jobs/{jobID}":                   rhf(RequireAuth, a.deleteJob),
		"POST /jobs/dev/create-orphan-candidate": rhf(RequireAuth, a.createOrphanCandidateJob),
		"POST /jobs/clear-completed":             rhf(RequireAuth, a.clearCompletedJobs),
		"POST /jobs/clear-all":                   rhf(RequireAuth, a.clearAllJobs),
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

	// If we have a Unix socket configured, create an unauthenticated mux for it
	var unixMux *http.ServeMux
	if config.UnixSocketPath != "" {
		unixMux = http.NewServeMux()
	}

	for p, h := range routes {
		if h.auth_state != NoAuth {
			a.mux.HandleFunc(p, authReq(dbx, sm, p, h.auth_state, h.handler))
		} else {
			a.mux.HandleFunc(p, h.handler)
		}
		if unixMux != nil {
			unixMux.HandleFunc(p, h.handler) // no auth on unix socket
		}
	}

	a.unixMux = unixMux

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
	unixMux   *http.ServeMux
}

func (t api) Run(started, stopped chan bool, stop chan context.Context) error {
	go func() {
		handler := cors.AllowAll().Handler(t.mux)
		srv := &http.Server{Addr: fmt.Sprintf("%s:%d", t.config.Bind, t.config.Port), Handler: handler}
		// Start TCP server
		go func() {
			if err := srv.ListenAndServe(); err != http.ErrServerClosed {
				log.Fatalf("HTTP server public ListenAndServe: %v", err)
			}
		}()

		// If unix socket enabled, start that server too
		if t.unixMux != nil {
			go func() {
				// Ensure socket does not already exist
				_ = os.Remove(t.config.UnixSocketPath)
				ln, err := net.Listen("unix", t.config.UnixSocketPath)
				if err != nil {
					log.Fatalf("HTTP unix listen: %v", err)
				}
				os.Chmod(t.config.UnixSocketPath, 0660)
				srvUnix := &http.Server{Handler: t.unixMux}
				if err := srvUnix.Serve(ln); err != http.ErrServerClosed {
					log.Fatalf("HTTP server unix Serve: %v", err)
				}
			}()
		}

		started <- true
		ctx := <-stop
		srv.Shutdown(ctx)
		stopped <- true
	}()
	return nil
}
