package dogeboxd

type ServerConfig struct {
	PupDir  string
	NixDir  string
	Bind    string
	Port    int
	Verbose bool
}
