package main

import (
	"flag"
	"log"
	"os"
	"path/filepath"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
	"github.com/dogeorg/dogeboxd/pkg/system"
)

func main() {
	// Create and load our config, then hand over to server.go

	var port int
	var bind string
	var dataDir string
	var nixDir string
	var uiDir string
	var uiPort int
	var containerLogDir string
	var internalPort int
	var verbose bool
	var help bool
	var forcedRecovery bool
	var dangerousDevMode bool
	var reflectorHost string

	flag.IntVar(&port, "port", 8080, "REST API Port")
	flag.StringVar(&bind, "addr", "127.0.0.1", "Address to bind to")
	flag.StringVar(&dataDir, "data", "/opt/dogebox", "Directory to write configuration files to")
	flag.StringVar(&nixDir, "nix", "/etc/nixos/dogebox", "Directory to write dogebox-specific nix configuration to")
	flag.StringVar(&uiDir, "uidir", "../dpanel/src", "Directory to find admin UI (dpanel)")
	flag.StringVar(&containerLogDir, "containerlogdir", "/var/log/containers", "Directory to write container logs to")
	flag.IntVar(&uiPort, "uiport", 8081, "Port for serving admin UI (dpanel)")
	flag.IntVar(&internalPort, "internal-port", 80, "Internal Router Port")
	flag.BoolVar(&forcedRecovery, "force-recovery", false, "Force recovery mode")
	flag.BoolVar(&dangerousDevMode, "danger-dev", false, "Enable dangerous development mode")
	flag.StringVar(&reflectorHost, "reflector-host", "https://reflector.dogebox.org", "Reflector host used for initial setup")
	flag.BoolVar(&verbose, "v", false, "Be verbose")
	flag.BoolVar(&help, "h", false, "Get help")
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}

	// Check if datadir exists and create if not
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		log.Printf("Specified datadir %s does not exist, creating it", dataDir)
		os.MkdirAll(dataDir, 0755)
	}

	tmpDir := filepath.Join(dataDir, "tmp")
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		log.Printf("Tmp directory %s does not exist, creating it", tmpDir)
		os.MkdirAll(tmpDir, 0755)
	}

	if _, err := os.Stat(containerLogDir); os.IsNotExist(err) {
		log.Printf("Container log directory %s does not exist, creating it", containerLogDir)
		err := os.MkdirAll(containerLogDir, 0750)
		if err != nil {
			log.Fatalf("Failed to create container log directory: %v", err)
		}
	}

	// Check if nixdir exists and create if not
	if _, err := os.Stat(nixDir); os.IsNotExist(err) {
		log.Printf("Specified nixdir %s does not exist, creating it", nixDir)
		err := os.MkdirAll(nixDir, 0755)
		if err != nil {
			log.Fatalf("Failed to create nixdir: %v", err)
		}
	}

	stateManager := system.NewStateManager(dataDir)
	err := stateManager.Load()
	if err != nil {
		log.Fatalf("Failed to load Dogeboxd system state: %+v", err)
	}

	recoveryMode := system.ShouldEnterRecovery(dataDir, stateManager)
	if forcedRecovery {
		recoveryMode = true
	}

	if recoveryMode {
		if err := system.DidEnterRecovery(dataDir); err != nil {
			log.Printf("Failed to call DidEnterRecovery: %+v", err)
		}

		log.Println("********************************************************************************")
		log.Println("************************ ENTERING DOGEBOX RECOVERY MODE ************************")
		log.Println("********************************************************************************")
	} else {
		if err := system.UnforceRecoveryNextBoot(dataDir); err != nil {
			log.Printf("Failed to call UnforceRecoveryNextBoot: %+v", err)
		}
	}

	if dangerousDevMode {
		log.Println("********************************************************************************")
		log.Println("******************************* DEV MODE ***************************************")
		log.Println("********************************************************************************")
	}

	config := dogeboxd.ServerConfig{
		Port:            port,
		Bind:            bind,
		DataDir:         dataDir,
		TmpDir:          tmpDir,
		NixDir:          nixDir,
		ContainerLogDir: containerLogDir,
		Verbose:         verbose,
		Recovery:        recoveryMode,
		UiDir:           uiDir,
		UiPort:          uiPort,
		InternalPort:    internalPort,
		DevMode:         dangerousDevMode,
		ReflectorHost:   reflectorHost,
	}

	srv := Server(stateManager, config)
	srv.Start()
}
