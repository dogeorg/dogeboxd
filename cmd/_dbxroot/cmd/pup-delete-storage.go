package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var deleteStorageCmd = &cobra.Command{
	Use:   "delete-storage",
	Short: "Delete storage for a pup",
	Long: `Delete storage for a pup by providing its ID and data directory.
This command requires --pupId and --data-dir flags.

Example:
  pup delete-storage --pupId mypup123 --data-dir /absolute/path/to/data`,
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

		fmt.Printf("Deleting storage for pup with ID: %s at %s\n", pupId, dataDir)

		storagePath := filepath.Join(dataDir, "pups", "storage", pupId)
		if err := os.RemoveAll(storagePath); err != nil {
			fmt.Printf("Error deleting storage directory: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Storage directory deleted at: %s\n", storagePath)
	},
}

func init() {
	pupCmd.AddCommand(deleteStorageCmd)

	deleteStorageCmd.Flags().StringP("pupId", "p", "", "ID of the pup to delete storage for (required, alphanumeric only)")
	deleteStorageCmd.MarkFlagRequired("pupId")

	deleteStorageCmd.Flags().StringP("data-dir", "d", "", "Absolute path to the data directory (required)")
	deleteStorageCmd.MarkFlagRequired("data-dir")
}
