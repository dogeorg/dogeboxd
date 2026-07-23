package lifecycle

import (
	"fmt"
	"log"
	"os"
	"os/exec"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
)

var _ dogeboxd.LifecycleManager = &LifecycleManagerLinux{}

type LifecycleManagerLinux struct {
	config dogeboxd.ServerConfig
}

var runRootCommand = func(action string) error {
	return exec.Command("_dbxroot", action).Run()
}

func (t LifecycleManagerLinux) Reboot() {
	if t.config.DevMode {
		log.Printf("In dev mode: Not rebooting, but killing service to make it obvious.")
		os.Exit(0)
		return
	}

	if err := runRootCommand("reboot"); err != nil {
		fmt.Printf("Failed to execute reboot command: %v\n", err)
	}
}

func (t LifecycleManagerLinux) Shutdown() {
	if t.config.DevMode {
		log.Printf("In dev mode: Not shutting down, but killing service to make it obvious.")
		os.Exit(0)
		return
	}

	if err := runRootCommand("shutdown"); err != nil {
		fmt.Printf("Failed to execute shutdown command: %v\n", err)
	}
}
