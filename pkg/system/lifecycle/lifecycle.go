package lifecycle

import (
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewLifecycleManager(config dogeboxd.ServerConfig) dogeboxd.LifecycleManager {
	// TODO: Do some discovery
	return LifecycleManagerLinux{config: config}
}
