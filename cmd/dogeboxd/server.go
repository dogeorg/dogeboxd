package main

import (
	_ "embed"
	"encoding/json"
	"log"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/conductor"
	"github.com/dogeorg/dogeboxd/pkg/sources"
	"github.com/dogeorg/dogeboxd/pkg/system"
)

//go:embed pup.json
var dogeboxManifestFile []byte

type server struct {
	config dogeboxd.ServerConfig
}

func Server(config dogeboxd.ServerConfig) server {
	return server{config}
}

func (t server) Start() {
	/* ----------------------------------------------------------------------- */
	// Establish the PUP manifest index so we have software to manage:

	manifest := dogeboxd.NewManifestIndex()

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

	/* ----------------------------------------------------------------------- */
	// Set up our system interfaces so we can talk to the host OS
	systemUpdater := system.NewSystemUpdater(t.config)
	systemMonitor := system.NewSystemMonitor(t.config)
	journalReader := system.NewJournalReader(t.config)

	/* ----------------------------------------------------------------------- */
	// Set up Dogeboxd, the beating heart of the beast

	dbx := dogeboxd.NewDogeboxd(t.config.PupDir, manifest, systemUpdater, systemMonitor, journalReader)

	/* ----------------------------------------------------------------------- */
	// Setup our external APIs. REST, Websockets

	wsh := dogeboxd.NewWSRelay(dbx.Changes)
	rest := dogeboxd.RESTAPI(t.config, dbx, wsh)

	/* ----------------------------------------------------------------------- */
	// Create a conductor to manage all the above services startup/shutdown

	var c *conductor.Conductor

	if t.config.Verbose {
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
	c.Service("System Updater", systemUpdater)
	c.Service("System Monitor", systemMonitor)
	c.Service("WSock Relay", wsh)
	c.Service("REST API", rest)
	// c.Service("Watcher", NewWatcher(t.state, t.config.PupDir))
	<-c.Start()
}
