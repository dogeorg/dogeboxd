package dogeboxd

import "github.com/dogeorg/dogeboxd/pkg/conductor"

type ServerConfig struct {
	PupDir  string
	Bind    string
	Port    int
	Verbose bool
}

type server struct {
	state  State
	config ServerConfig
}

func Server(config ServerConfig) server {
	s := LoadState(config.PupDir)
	return server{s, config}
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

	c.Service("Watcher", NewWatcher(t.state, t.config.PupDir))
	c.Service("REST API", RESTAPI(t.config, t.state))
	<-c.Start()
}
