package lifecycle

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

var _ dogeboxd.LifecycleManager = &LifecycleManagerLinux{}

type LifecycleManagerLinux struct {
	config dogeboxd.ServerConfig
}

func (t LifecycleManagerLinux) Reboot() {
	if t.config.DevMode {
		log.Printf("In dev mode: Not rebooting, but killing service to make it obvious.")
		os.Exit(0)
		return
	}

	cmd := exec.Command("sudo", "_dbxroot", "reboot")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to execute reboot command: %v\n", err)
	}
}

func (t LifecycleManagerLinux) Shutdown() {
	if t.config.DevMode {
		log.Printf("In dev mode: Not shutting down, but killing service to make it obvious.")
		os.Exit(0)
		return
	}

	cmd := exec.Command("sudo", "_dbxroot", "shutdown")
	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to execute reboot command: %v\n", err)
	}
}
