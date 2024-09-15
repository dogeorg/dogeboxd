package lifecycle

import (
	"fmt"
	"os/exec"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.LifecycleManager = &LifecycleManagerLinux{}

type LifecycleManagerLinux struct{}

func (t LifecycleManagerLinux) Reboot() {
	cmd := exec.Command("_dbxroot", "reboot")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to execute reboot command: %v\n", err)
	}
}

func (t LifecycleManagerLinux) Shutdown() {
	cmd := exec.Command("_dbxroot", "shutdown")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to execute reboot command: %v\n", err)
	}
}
