package main

import (
	"flag"
	"log"
	"os"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
)

func main() {
	// Create and load our config, then hand over to server.go

	var port int
	var bind string
	var pupDir string
	var nixDir string
	var uiDir string
	var uiPort int
	var verbose bool
	var help bool
	var forcedRecovery bool

	flag.IntVar(&port, "port", 8080, "REST API Port")
	flag.StringVar(&bind, "addr", "127.0.0.1", "Address to bind to")
	flag.StringVar(&pupDir, "pups", "./pups", "Directory to find local pups")
	flag.StringVar(&nixDir, "nix", "/etc/nixos/dogebox", "Directory to write dogebox-specific nix configuration to")
	flag.StringVar(&uiDir, "uidir", "../dpanel/src", "Directory to find admin UI (dpanel)")
	flag.IntVar(&uiPort, "uiport", 8081, "Port for serving admin UI (dpanel)")
	flag.BoolVar(&forcedRecovery, "force-recovery", false, "Force recovery mode")
	flag.BoolVar(&verbose, "v", false, "Be verbose")
	flag.BoolVar(&help, "h", false, "Get help")
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}

	stateManager := system.NewStateManager()
	err := stateManager.Load()
	if err != nil {
		log.Fatalf("Failed to load Dogeboxd system state: %+v", err)
	}

	recoveryMode := system.ShouldEnterRecovery(stateManager)
	if forcedRecovery {
		recoveryMode = true
	}

	if recoveryMode {
		log.Println("********************************************************************************")
		log.Println("************************ ENTERING DOGEBOX RECOVERY MODE ************************")
		log.Println("********************************************************************************")
	}

	config := dogeboxd.ServerConfig{
		Port:     port,
		Bind:     bind,
		PupDir:   pupDir,
		NixDir:   nixDir,
		Verbose:  verbose,
		Recovery: recoveryMode,
		UiDir:    uiDir,
		UiPort:   uiPort,
	}

	srv := Server(stateManager, config)
	srv.Start()
}
