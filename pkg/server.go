package dogeboxd

import "github.com/dogeorg/dogeboxd/pkg/conductor"

type ServerConfig struct {
	PupDir  string
	Bind    string
	Port    int
	Verbose bool
}

type server struct {
	config ServerConfig
}

func Server(config ServerConfig) server {
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

	dbx := NewDogeboxd(t.config.PupDir)
	c.Service("Dogeboxd", dbx)
	c.Service("REST API", RESTAPI(t.config, dbx))
	// c.Service("Watcher", NewWatcher(t.state, t.config.PupDir))
	<-c.Start()
}
