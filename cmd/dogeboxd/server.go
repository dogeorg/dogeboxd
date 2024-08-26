package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/dogeorg/dogeboxd/pkg/sources"
	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/dogeorg/dogeboxd/pkg/system/lifecycle"
	"github.com/dogeorg/dogeboxd/pkg/system/network"
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

func (t server) loadManifest() dogeboxd.ManifestIndex {
	// Establish the PUP manifest index so we have software to manage:
	manifest := dogeboxd.NewManifestIndex()

	if t.config.Recovery {
		// Do nothing if we're in recovery mode.
		log.Printf("In recovery mode: not loading manifests.")
		return manifest
	}

	// Setup the 'local' source that represents
	// development pups on the local Filesystem
	localSource := sources.NewLocalFileSource("local", "Local Filesystem", t.config.PupDir)
	manifest.AddSource("local", localSource)

	// Set up the 'internal' source that represents
	// the dogeboxd to itself
	internalSource := sources.NewInternalSource()

	// Create a manifestifest for Dogebox itself from ./pup.json
	var dbMan dogeboxd.PupManifest
	err := json.Unmarshal(dogeboxManifestFile, &dbMan)
	if err != nil {
		log.Fatalln("Couldn't load Dogeboxd's own manifestifest")
	}
	internalSource.AddManifest(dbMan)

	manifest.AddSource("internal", internalSource)

	return manifest
}

func (t server) Start() {
	/* ----------------------------------------------------------------------- */
	manifest := t.loadManifest()

	/* ----------------------------------------------------------------------- */
	// Set up our system interfaces so we can talk to the host OS
	networkManager := network.NewNetworkManager(t.sm)
	lifecycleManager := lifecycle.NewLifecycleManager()

	systemUpdater := system.NewSystemUpdater(t.config, networkManager)
	systemMonitor := system.NewSystemMonitor(t.config)
	journalReader := system.NewJournalReader(t.config)

	/* ----------------------------------------------------------------------- */
	// Set up PupManager and load the state for all installed pups
	//

	pups, err := dogeboxd.NewPupManager(t.config.PupDir)
	if err != nil {
		log.Fatalf("Failed to load Pup state: %+v", err)
	}
	fmt.Printf("Loading pups from %s\n", t.config.PupDir)

	for k, p := range pups.GetStateMap() {
		fmt.Printf("pups %s:\n %+v\n", k, p)
	}

	/* ----------------------------------------------------------------------- */
	// Set up Dogeboxd, the beating heart of the beast

	dbx := dogeboxd.NewDogeboxd(t.sm, pups, manifest, systemUpdater, systemMonitor, journalReader, networkManager, lifecycleManager)

	/* ----------------------------------------------------------------------- */
	// Setup our external APIs. REST, Websockets

	wsh := dogeboxd.NewWSRelay(t.config, dbx.Changes)
	rest := dogeboxd.RESTAPI(t.config, dbx, manifest, pups, wsh)
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
