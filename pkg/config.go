package dogeboxd

type ServerConfig struct {
	DataDir          string
	TmpDir           string
	NixDir           string
	ContainerLogDir  string
	Bind             string
	Port             int
	InternalPort     int
	Verbose          bool
	Recovery         bool
	UiDir            string
	UiPort           int
	DevMode          bool
	ReflectorHost    string
	DisableReflector bool
}

func GetSystemEnvironmentVariablesForContainer() map[string]string {
	return map[string]string{
		"DBX_HOST": "10.69.0.1",
		"DBX_PORT": "80",
	}
}
