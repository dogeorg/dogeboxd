package main

import (
	"flag"
	"log"
	"os"

	"github.com/dogeorg/dogeboxd/pkg/system"
	"github.com/dogeorg/dogeboxd/pkg/system/lifecycle"
)

func main() {
	var dataDir string
	var help bool
	flag.StringVar(&dataDir, "data", "/etc/dogebox", "Directory to write configuration files to")
	flag.BoolVar(&help, "h", false, "Get help")

	if help {
		flag.Usage()
		os.Exit(0)
	}

	hasFile := system.HasForceRecoveryFile(dataDir)

	if hasFile {
		log.Println("Will already enter recovery mode next boot.")
		os.Exit(0)
	}

	if err := system.ForceRecoveryNextBoot(dataDir); err != nil {
		log.Println("Failed to write file.")
		os.Exit(1)
	}

	log.Println("Wrote flag, rebooting..")

	lifecycleManager := lifecycle.NewLifecycleManager()
	lifecycleManager.Reboot()
}
