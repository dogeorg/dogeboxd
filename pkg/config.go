package dogeboxd

type ServerConfig struct {
	DataDir      string
	TmpDir       string
	NixDir       string
	Bind         string
	Port         int
	InternalPort int
	Verbose      bool
	Recovery     bool
	UiDir        string
	UiPort       int
	DevMode      bool
}
