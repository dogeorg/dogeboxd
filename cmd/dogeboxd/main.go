package main

import (
	"flag"
	"os"

	dogeboxd "github.com/dogeorg/dogeboxd/pkg"
)

func main() {
	// Create and load our config, then hand over to server.go

	var port int
	var bind string
	var pupDir string
	var nixDir string
	var verbose bool
	var help bool

	flag.IntVar(&port, "port", 8080, "REST API Port")
	flag.StringVar(&bind, "addr", "127.0.0.1", "Address to bind to")
	flag.StringVar(&pupDir, "pups", "./pups", "Directory to find local pups")
	flag.StringVar(&nixDir, "nix", "./nix", "Directory to find nix ??")
	flag.BoolVar(&verbose, "v", false, "Be verbose")
	flag.BoolVar(&help, "h", false, "Get help")
	flag.Parse()

	if help {
		flag.Usage()
		os.Exit(0)
	}

	config := dogeboxd.ServerConfig{
		Port:    port,
		Bind:    bind,
		PupDir:  pupDir,
		NixDir:  nixDir,
		Verbose: verbose,
	}

	srv := Server(config)
	srv.Start()
}
