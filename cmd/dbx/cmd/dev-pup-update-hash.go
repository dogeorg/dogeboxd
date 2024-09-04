package cmd

import (
	"crypto/sha256"
	_ "embed"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/dogeorg/dogeboxd/pkg/pup"
	"github.com/spf13/cobra"
)

var devPupUpdateHashCmd = &cobra.Command{
	Use:   "update-hash",
	Short: "Easy command to update the nixHash key in your pup manifest",
	Run: func(cmd *cobra.Command, args []string) {
		pupDir, err := cmd.Flags().GetString("pupDir")
		if err != nil {
			log.Fatalf("Error getting pupDir flag: %v", err)
		}

		if pupDir == "" {
			pupDir, err = os.Getwd()
			if err != nil {
				log.Fatalf("Error getting current working directory: %v", err)
			}
		}

		manifestPath := filepath.Join(pupDir, "manifest.json")

		manifestFile, err := os.ReadFile(manifestPath)
		if err != nil {
			log.Fatalf("Error reading manifest file: %v", err)
		}
		var manifest pup.PupManifest
		err = json.Unmarshal(manifestFile, &manifest)
		if err != nil {
			log.Fatalf("Error unmarshalling manifest file: %v", err)
		}

		nixFilePath := filepath.Join(pupDir, manifest.Container.Build.NixFile)

		nixFile, err := os.ReadFile(nixFilePath)
		if err != nil {
			log.Fatalf("Error reading nix file: %v", err)
		}
		nixHash := sha256.Sum256(nixFile)
		manifest.Container.Build.NixFileSha256 = fmt.Sprintf("%x", nixHash)

		updatedManifest, err := json.MarshalIndent(manifest, "", "  ")
		if err != nil {
			log.Fatalf("Error marshalling updated manifest: %v", err)
		}

		err = os.WriteFile(manifestPath, updatedManifest, 0644)
		if err != nil {
			log.Fatalf("Error writing updated manifest to file: %v", err)
		}

		cmd.Printf("Updated manifest.json with hash sha256(%s)=%s\n", manifest.Container.Build.NixFile, fmt.Sprintf("%x", nixHash))
	},
}

func init() {
	devPupUpdateHashCmd.Flags().StringP("pupDir", "p", "", "Directory of the pup you want to modify")
	devPupCmd.AddCommand(devPupUpdateHashCmd)
}
