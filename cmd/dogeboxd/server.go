package main

import (
	_ "embed"
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
	source "github.com/dogeorg/dogeboxd/pkg/sources"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/dogeorg/dogeboxd/pkg/system/lifecycle"
	"github.com/dogeorg/dogeboxd/pkg/system/network"
	"github.com/dogeorg/dogeboxd/pkg/system/nix"
	"github.com/dogeorg/dogeboxd/pkg/web"
)

//go:embed pup.json
var dogeboxManifestFile []byte

type server struct {
	sm     dogeboxd.StateManager
	config dogeboxd.ServerConfig
}

func Server(sm dogeboxd.StateManager, config dogeboxd.ServerConfig) server {
	return server{
		sm:     sm,
		config: config,
	}
}

func (t server) Start() {
	systemMonitor := system.NewSystemMonitor(t.config)

	pups, err := dogeboxd.NewPupManager(t.config.DataDir, t.config.TmpDir, systemMonitor)
	if err != nil {
		log.Fatalf("Failed to load Pup state: %+v", err)
	}

	// Set up a doge key manager connection
	dkm := dogeboxd.NewDKMManager()

	sourceManager := source.NewSourceManager(t.config, t.sm, pups)
	pups.SetSourceManager(sourceManager)
	nixManager := nix.NewNixManager(t.config, pups)

	// Set up our system interfaces so we can talk to the host OS
	networkManager := network.NewNetworkManager(nixManager, t.sm)
	lifecycleManager := lifecycle.NewLifecycleManager(t.config)

	systemUpdater := system.NewSystemUpdater(t.config, networkManager, nixManager, sourceManager, pups, t.sm, dkm)
	journalReader := system.NewJournalReader(t.config)
	logtailer := system.NewLogTailer(t.config)

	/* ----------------------------------------------------------------------- */
	// Set up PupManager and load the state for all installed pups
	//

	for k, p := range pups.GetStateMap() {
		fmt.Printf("pups %s:\n %+v\n", k, p)
	}

	// Check if we have pending reflector data to submit.
	if err := system.CheckAndSubmitReflectorData(t.config, networkManager); err != nil {
		log.Printf("Error checking and submitting reflector data: %v", err)
	}

	/* ----------------------------------------------------------------------- */
	// Set up Dogeboxd, the beating heart of the beast

	dbx := dogeboxd.NewDogeboxd(t.sm, pups, systemUpdater, systemMonitor, journalReader, networkManager, sourceManager, nixManager, logtailer)

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
	c.Service("Dogeboxd", dbx)
	c.Service("REST API", rest)
	c.Service("UI Server", ui)
	c.Service("System Updater", systemUpdater)

	if !t.config.Recovery {
		c.Service("System Monitor", systemMonitor)
		c.Service("WSock Relay", wsh)
		c.Service("Pup Manager", pups)
		c.Service("Internal Router", internalRouter)
		c.Service("Admin Router", adminRouter)
	}

	// c.Service("Watcher", NewWatcher(t.state, t.config.PupDir))
	<-c.Start()
}
