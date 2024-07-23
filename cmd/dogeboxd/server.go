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
var dbxManifestFile []byte

type server struct {
	config dogeboxd.ServerConfig
}

func Server(config dogeboxd.ServerConfig) server {
	return server{config}
}

func (t server) Start() {
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

	su := system.NewSystemUpdater(t.config)
	sm := system.NewSystemMonitor(t.config)
	j := system.NewJournalReader(t.config, "")

	// Setup the ManifestIndex which knows about
	// all available pups
	man := dogeboxd.NewManifestIndex()

	// Setup the 'local' source that represents
	// development pups on the local Filesystem
	localSource := sources.NewLocalFileSource("local", "Local Filesystem", t.config.PupDir)
	man.AddSource("local", localSource)

	// Set up the 'internal' source that represents
	// the dogeboxd to itself
	internalSource := sources.NewInternalSource()

	// Create a Manifest for Dogebox itself from ./pup.json
	var dbMan dogeboxd.PupManifest
	err := json.Unmarshal(dbxManifestFile, &dbMan)
	if err != nil {
		log.Fatalln("Couldn't load Dogeboxd's own manifest")
	}
	internalSource.AddManifest(dbMan)

	man.AddSource("internal", internalSource)

	dbx := dogeboxd.NewDogeboxd(t.config.PupDir, man, su, sm)
	wsh := dogeboxd.NewWSRelay(dbx.Changes)
	c.Service("Dogeboxd", dbx)
	c.Service("System Manager", su)
	c.Service("System Monitor", sm)
	c.Service("Journal Reader", j)
	c.Service("WSock Relay", wsh)
	c.Service("REST API", dogeboxd.RESTAPI(t.config, dbx, wsh))
	// c.Service("Watcher", NewWatcher(t.state, t.config.PupDir))
	<-c.Start()
}
