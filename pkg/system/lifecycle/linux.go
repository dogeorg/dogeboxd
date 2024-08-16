package lifecycle

import (
	"os/exec"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.LifecycleManager = &LifecycleManagerLinux{}

type LifecycleManagerLinux struct{}

func (t LifecycleManagerLinux) Reboot() {
	exec.Command("shutdown")
}

func (t LifecycleManagerLinux) Shutdown() {
	exec.Command("reboot")
}
