package lifecycle

import (
	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func NewLifecycleManager() dogeboxd.LifecycleManager {
	// TODO: Do some discovery
	return LifecycleManagerLinux{}
}
