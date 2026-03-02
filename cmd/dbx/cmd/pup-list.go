package cmd

import (
	"encoding/gob"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	dogeboxd "github.com/Dogebox-WG/dogeboxd/pkg"
	"github.com/spf13/cobra"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List all installed pups",
	Long:  `List all installed pups with their name, ID, and status.`,
	Run: func(cmd *cobra.Command, args []string) {
		dataDir, err := cmd.Flags().GetString("dataDir")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting dataDir flag: %v\n", err)
			os.Exit(1)
		}

		pupDir := filepath.Join(dataDir, "pups")

		// Check if pup directory exists
		if _, err := os.Stat(pupDir); os.IsNotExist(err) {
			fmt.Println("No pups installed")
			return
		}

		// Find all .gob files
		files, err := os.ReadDir(pupDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error reading pup directory: %v\n", err)
			os.Exit(1)
		}

		var pupStates []dogeboxd.PupState
		for _, file := range files {
			if strings.HasSuffix(file.Name(), ".gob") {
				pupPath := filepath.Join(pupDir, file.Name())
				state, err := loadPupState(pupPath)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Warning: Failed to load pup from %s: %v\n", file.Name(), err)
					continue
				}
				pupStates = append(pupStates, state)
			}
		}

		if len(pupStates) == 0 {
			fmt.Println("No pups installed")
			return
		}

		// Print header
		fmt.Printf("%-30s %-32s %-15s\n", "NAME", "ID", "STATUS")
		fmt.Printf("%-30s %-32s %-15s\n", strings.Repeat("-", 30), strings.Repeat("-", 32), strings.Repeat("-", 15))

		// Print each pup
		for _, pup := range pupStates {
			status := getStatusDisplay(pup)
			fmt.Printf("%-30s %-32s %-15s\n", pup.Manifest.Meta.Name, pup.ID, status)
		}
	},
}

func loadPupState(path string) (dogeboxd.PupState, error) {
	var state dogeboxd.PupState

	file, err := os.Open(path)
	if err != nil {
		return state, err
	}
	defer file.Close()

	decoder := gob.NewDecoder(file)
	if err := decoder.Decode(&state); err != nil {
		if err == io.EOF {
			return state, fmt.Errorf("empty file")
		}
		return state, err
	}

	return state, nil
}

func getStatusDisplay(pup dogeboxd.PupState) string {
	// Primary status is based on Installation field
	status := string(pup.Installation)

	// Add running state if enabled
	if pup.Installation == dogeboxd.STATE_READY && pup.Enabled {
		status = "running"
	} else if pup.Installation == dogeboxd.STATE_READY && !pup.Enabled {
		status = "stopped"
	}

	// Add broken reason if broken
	if pup.Installation == dogeboxd.STATE_BROKEN && pup.BrokenReason != "" {
		status = fmt.Sprintf("broken (%s)", pup.BrokenReason)
	}

	return status
}

func init() {
	pupCmd.AddCommand(listCmd)

	listCmd.Flags().String("dataDir", "/opt/dogebox", "Path to dogeboxd data directory")
}
