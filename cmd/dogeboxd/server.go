package main

import (
	_ "embed"
	"encoding/json"
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
	config dogeboxd.ServerConfig
}

func Server(config dogeboxd.ServerConfig) server {
	return server{config}
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

func (t server) getInternalState() dogeboxd.InternalState {
	if t.config.Recovery {
		return dogeboxd.InternalState{}
	}

	return dogeboxd.InternalState{
		ActionCounter: 100000,
		InstalledPups: []string{"internal.dogeboxd"},
	}
}

func (t server) Start() {
	/* ----------------------------------------------------------------------- */
	manifest := t.loadManifest()

	/* ----------------------------------------------------------------------- */
	stateManager := system.NewStateManager(t.config)
	err := stateManager.Load()
	if err != nil {
		log.Fatalf("Failed to load Dogeboxd system state: %+v", err)
	}

	// Set up our system interfaces so we can talk to the host OS
	networkManager := network.NewNetworkManager(stateManager)
	lifecycleManager := lifecycle.NewLifecycleManager()

	systemUpdater := system.NewSystemUpdater(t.config, networkManager)
	systemMonitor := system.NewSystemMonitor(t.config)
	journalReader := system.NewJournalReader(t.config)

	internalState := t.getInternalState()

	/* ----------------------------------------------------------------------- */
	// Set up PupManager and load the state for all installed pups
	//

	pups, err := dogeboxd.NewPupManager(t.config.PupDir)
	if err != nil {
		log.Fatalf("Failed to load Pup state: %+v", err)
	}

	/* ----------------------------------------------------------------------- */
	// Set up Dogeboxd, the beating heart of the beast

	dbx := dogeboxd.NewDogeboxd(internalState, pups, manifest, systemUpdater, systemMonitor, journalReader, networkManager, lifecycleManager)

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
