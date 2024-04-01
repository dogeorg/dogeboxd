package dogeboxd

import "github.com/dogeorg/dogeboxd/pkg/conductor"

type ServerConfig struct {
	PupDir  string
	Bind    string
	Port    int
	Verbose bool
}

func Server(config ServerConfig) server {
	s := NewState()
	s.LoadLocalManifests(config.PupDir)
	return server{&s, config}
}

type server struct {
	state  *State
	config ServerConfig
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

	c.Service("REST API", RESTAPI(t.config, *t.state))
	<-c.Start()
}
