package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// This program is setuid so that it can be run by the dogeboxd user.
// Executing this program directly on NixOS systems will not work.
// Instead, run the wrapper that should be setuid @ /run/wrappers/bin/machinectlstop
// This wrapper is configured in dogeboxd's nix template system.nix

func main() {
	if syscall.Geteuid() != 0 {
		fmt.Fprintln(os.Stderr, "This program must be run as root.")
		fmt.Fprintln(os.Stderr, "Your system should automatically be set up for this to work.")
		fmt.Fprintln(os.Stderr, "If you're seeing this, please report it to the Dogebox team.")
		os.Exit(1)
	}

	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "Usage: machinectlstop pup-id")
		os.Exit(1)
	}

	pupID := os.Args[1]
	for _, char := range pupID {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9')) {
			fmt.Fprintln(os.Stderr, "Invalid pup-id. Only alphanumeric characters are allowed.")
			os.Exit(1)
		}
	}
	cmd := exec.Command("sudo", "machinectl", "stop", fmt.Sprintf("pup-%s", pupID))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Fprintln(os.Stderr, "Error executing nixos-rebuild switch:", err)
		os.Exit(1)
	}
}
