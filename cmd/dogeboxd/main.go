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
	var verbose bool
	var help bool
	var forcedRecovery bool

	flag.IntVar(&port, "port", 8080, "REST API Port")
	flag.StringVar(&bind, "addr", "127.0.0.1", "Address to bind to")
	flag.StringVar(&pupDir, "pups", "./pups", "Directory to find local pups")
	flag.StringVar(&nixDir, "nix", "./nix", "Directory to find nix ??")
	flag.BoolVar(&forcedRecovery, "force-recovery", false, "Force recovery mode")
	flag.BoolVar(&verbose, "v", false, "Be verbose")
	flag.BoolVar(&help, "h", false, "Get help")
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}

	recoveryMode := system.ShouldEnterRecovery()
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
	}

	srv := Server(config)
	srv.Start()
}
