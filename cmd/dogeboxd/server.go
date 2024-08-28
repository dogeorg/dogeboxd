package main

import (
	_ "embed"
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/dogeorg/dogeboxd/pkg/repository"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/dogeorg/dogeboxd/pkg/system/lifecycle"
	"github.com/dogeorg/dogeboxd/pkg/system/network"
	"github.com/dogeorg/dogeboxd/pkg/system/nix"
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
	repositoryManager := repository.NewRepositoryManager(t.sm)
	nixManager := nix.NewNixManager(t.config)

	// Set up our system interfaces so we can talk to the host OS
	networkManager := network.NewNetworkManager(t.sm)
	lifecycleManager := lifecycle.NewLifecycleManager()

	systemUpdater := system.NewSystemUpdater(t.config, networkManager, nixManager)
	systemMonitor := system.NewSystemMonitor(t.config)
	journalReader := system.NewJournalReader(t.config)

	/* ----------------------------------------------------------------------- */
	// Set up PupManager and load the state for all installed pups
	//

	pups, err := dogeboxd.NewPupManager(t.config.DataDir)
	if err != nil {
		log.Fatalf("Failed to load Pup state: %+v", err)
	}

	for k, p := range pups.GetStateMap() {
		fmt.Printf("pups %s:\n %+v\n", k, p)
	}

	/* ----------------------------------------------------------------------- */
	// Set up Dogeboxd, the beating heart of the beast

	dbx := dogeboxd.NewDogeboxd(t.sm, pups, systemUpdater, systemMonitor, journalReader, networkManager, lifecycleManager, repositoryManager)

	/* ----------------------------------------------------------------------- */
	// Setup our external APIs. REST, Websockets

	wsh := dogeboxd.NewWSRelay(t.config, dbx.Changes)
	rest := dogeboxd.RESTAPI(t.config, dbx, pups, wsh)
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

	if !t.config.Recovery {
		c.Service("System Updater", systemUpdater)
		c.Service("System Monitor", systemMonitor)
		c.Service("WSock Relay", wsh)
	}

	// c.Service("Watcher", NewWatcher(t.state, t.config.PupDir))
	<-c.Start()
}
