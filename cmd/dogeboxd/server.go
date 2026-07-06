package main

import (
	_ "embed"
	"fmt"
	"log"
	"sync/atomic"
	"time"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/Dogebox-WG/dogeboxd/pkg/conductor"
	"github.com/Dogebox-WG/dogeboxd/pkg/pup"
	source "github.com/Dogebox-WG/dogeboxd/pkg/sources"
	"github.com/Dogebox-WG/dogeboxd/pkg/system"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/lifecycle"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/network"
	"github.com/Dogebox-WG/dogeboxd/pkg/system/nix"
	"github.com/Dogebox-WG/dogeboxd/pkg/web"
)

//go:embed pup.json
var dogeboxManifestFile []byte

type server struct {
	store  *dogeboxd.StoreManager
	sm     dogeboxd.StateManager
	config dogeboxd.ServerConfig
}

func Server(sm dogeboxd.StateManager, store *dogeboxd.StoreManager, config dogeboxd.ServerConfig) server {
	return server{
		store:  store,
		sm:     sm,
		config: config,
	}
}

func (t server) Start() {
	systemMonitor := system.NewSystemMonitor(t.config)

	pups, err := pup.NewPupManager(t.config, systemMonitor)
	if err != nil {
		log.Fatalf("Failed to load Pup state: %+v", err)
	}

	// Set up a doge key manager connection
	dkm := dogeboxd.NewDKMManager()

	sourceManager := source.NewSourceManager(t.config, t.sm, pups)
	pups.SetSourceManager(sourceManager)

	// Add hook to post nix rebuild
	var dbxReady uint32
	var dbx dogeboxd.Dogeboxd
	postRebuild := func() {
		if atomic.LoadUint32(&dbxReady) == 0 {
			return
		}
		if !t.sm.Get().Dogebox.InitialState.HasFullyConfigured {
			log.Printf("Skipping post-rebuild nix cache update because initial bootstrap will reboot shortly and would interrupt the cache warm") // Instead, we'll warm the cache on setup.
			return
		}
		go dbx.AddAction(dogeboxd.UpdateNixCache{})
	}

	nixManager := nix.NewNixManager(t.config, pups, postRebuild)

	// Set up our system interfaces so we can talk to the host OS
	networkManager := network.NewNetworkManager(nixManager, t.sm)
	lifecycleManager := lifecycle.NewLifecycleManager(t.config)

	systemUpdater := system.NewSystemUpdater(t.config, networkManager, nixManager, sourceManager, pups, t.sm, lifecycleManager, dkm)
	journalReader := system.NewJournalReader(t.config)
	logtailer := system.NewLogTailer()

	/* ----------------------------------------------------------------------- */
	// Set up PupManager and load the state for all installed pups
	//

	for k, p := range pups.GetStateMap() {
		logoInfo := "(none)"
		if len(p.LogoBase64) > 0 {
			logoInfo = fmt.Sprintf("(%d bytes)", len(p.LogoBase64))
		}
		fmt.Printf("pup %s: Name=%s, Version=%s, Installation=%s, Logo=%s\n",
			k, p.Manifest.Meta.Name, p.Version, p.Installation, logoInfo)
	}

	// Check if we have pending reflector data to submit.
	if err := system.CheckAndSubmitReflectorData(t.config, networkManager); err != nil {
		log.Printf("Error checking and submitting reflector data: %v", err)
	}

	/* ----------------------------------------------------------------------- */
	// Set up Dogeboxd, the beating heart of the beast

	// Create Dogeboxd instance
	dbx = dogeboxd.NewDogeboxd(t.sm, pups, systemUpdater, systemMonitor, journalReader, networkManager, sourceManager, nixManager, logtailer, pups, &t.config)

	// Create JobManager
	jobManager := dogeboxd.NewJobManager(t.store, &dbx)
	dbx.SetJobManager(jobManager)
	atomic.StoreUint32(&dbxReady, 1)

	if reconciled, err := jobManager.ReconcileCompletedSystemUpdateJobs(); err == nil && reconciled > 0 {
		log.Printf("Reconciled %d interrupted system update jobs to completed after restart", reconciled)
	}

	if cleared, err := jobManager.ClearInterruptedSystemJobs(); err == nil && cleared > 0 {
		log.Printf("Cleaned up %d interrupted system jobs from previous run", cleared)
	}

	// Clean up any orphaned jobs from previous runs (stuck in queued/in_progress)
	// before startup migrations inspect active work and decide whether to queue again.
	if cleared, err := jobManager.ClearOrphanedJobs(30 * time.Minute); err == nil && cleared > 0 {
		log.Printf("Cleaned up %d orphaned jobs from previous run", cleared)
	}

	if t.sm.Get().Dogebox.InitialState.HasFullyConfigured {
		go func() {
			if t.checkAndPerformPostUpgradeMigrations(dbx) {
				return
			}

			jobID := dbx.AddAction(dogeboxd.UpdateNixCache{})
			log.Printf("Queued startup nix cache update job: %s", jobID)
		}()
	}

	//No need to show welcome screen if any pups are already installed (may have just done a system update or something similar)
	if len(pups.GetStateMap()) > 0 {
		state := t.sm.Get()
		state.Dogebox.Flags.IsFirstTimeWelcomeComplete = true
		t.sm.SetDogebox(state.Dogebox)
	}

	/* ----------------------------------------------------------------------- */
	// Setup our external APIs. REST, Websockets

	wsh := web.NewWSRelay(t.config, dbx.Changes)
	adminRouter := web.NewAdminRouter(t.config, pups)
	rest := web.RESTAPI(t.config, t.sm, dbx, pups, sourceManager, lifecycleManager, nixManager, dkm, wsh)
	internalRouter := web.NewInternalRouter(t.config, dbx, pups, dkm)
	ui := dogeboxd.ServeUI(t.config)

	/* ----------------------------------------------------------------------- */
	// Create a conductor to manage all the above services startup/shutdown

	var c *conductor.Conductor

	if t.config.Verbose || t.config.Recovery {
		c = conductor.NewConductor(
			conductor.HookSignals(),
			conductor.Noisy(),
		)
	} else {
		c = conductor.NewConductor(
			conductor.HookSignals(),
		)
	}
	c.Service("Store", t.store)
	c.Service("Dogeboxd", dbx)
	c.Service("REST API", rest)
	c.Service("UI Server", ui)
	c.Service("System Updater", systemUpdater)
	c.Service("WSock Relay", wsh)

	if !t.config.Recovery {
		c.Service("System Monitor", systemMonitor)
		c.Service("Pup Manager", pups)
		c.Service("Internal Router", internalRouter)
		c.Service("Admin Router", adminRouter)
	}

	// c.Service("Watcher", NewWatcher(t.state, t.config.PupDir))
	<-c.Start()
}
