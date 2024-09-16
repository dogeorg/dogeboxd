package cmd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

const (
	storageDirPerm   fs.FileMode = 0755
	containerUserId  int         = 420
	containerGroupId int         = 69
)

var createStorageCmd = &cobra.Command{
	Use:   "create-storage",
	Short: "Create storage for a pup",
	Long: `Create storage for a pup by providing its ID and data directory.
This command requires --pupId and --data-dir flags.

Example:
  pup create-storage --pupId mypup123 --data-dir /absolute/path/to/data`,
	Run: func(cmd *cobra.Command, args []string) {
		pupId, _ := cmd.Flags().GetString("pupId")
		dataDir, _ := cmd.Flags().GetString("data-dir")

		if !IsAlphanumeric(pupId) {
			fmt.Println("Error: pupId must contain only alphanumeric characters")
			os.Exit(1)
		}

		if !IsAbsolutePath(dataDir) {
			fmt.Println("Error: data-dir must be an absolute path")
			os.Exit(1)
		}

		fmt.Printf("Creating storage for pup with ID: %s at %s\n", pupId, dataDir)

		storagePath := filepath.Join(dataDir, "pups", "storage", pupId)
		if err := os.MkdirAll(storagePath, storageDirPerm); err != nil {
			fmt.Printf("Error creating storage directory: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Storage directory created at: %s\n", storagePath)
		if err := os.Chown(storagePath, containerUserId, containerGroupId); err != nil {
			fmt.Printf("Error changing ownership of storage directory: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Storage directory ownership changed to %d:%d\n", containerUserId, containerGroupId)
	},
}

func init() {
	pupCmd.AddCommand(createStorageCmd)

	createStorageCmd.Flags().StringP("pupId", "p", "", "ID of the pup to create storage for (required, alphanumeric only)")
	createStorageCmd.MarkFlagRequired("pupId")

	createStorageCmd.Flags().StringP("data-dir", "d", "", "Absolute path to the data directory (required)")
	createStorageCmd.MarkFlagRequired("data-dir")
}
