package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/dogeorg/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

var writeKeyCmd = &cobra.Command{
	Use:   "write-key",
	Short: "Write a key to a pup's storage",
	Long: `Write a key to a pup's storage directory.
This command requires --pupId, --data-dir, --key-file and --data flags.

Example:
  pup write-key --pupId 1234 --data-dir /absolute/path/to/data --key-file delegate.key --data "your_secret_key"`,
	Run: func(cmd *cobra.Command, args []string) {
		pupId, _ := cmd.Flags().GetString("pupId")
		dataDir, _ := cmd.Flags().GetString("data-dir")
		keyFile, _ := cmd.Flags().GetString("key-file")
		data, _ := cmd.Flags().GetString("data")

		if !utils.IsAlphanumeric(pupId) {
			fmt.Println("Error: pupId must contain only alphanumeric characters")
			os.Exit(1)
		}

		if !utils.IsAbsolutePath(dataDir) {
			fmt.Println("Error: data-dir must be an absolute path")
			os.Exit(1)
		}

		storagePath := filepath.Join(dataDir, "pups", "storage", pupId)

		if _, err := os.Stat(storagePath); os.IsNotExist(err) {
			fmt.Println("Error: Storage directory does not exist. Please create it first.")
			os.Exit(1)
		}

		keyFilePath := filepath.Join(storagePath, keyFile)

		if err := ioutil.WriteFile(keyFilePath, []byte(data), 0644); err != nil {
			fmt.Printf("Error writing key file: %v\n", err)
			os.Exit(1)
		}

		if err := os.Chown(keyFilePath, containerUserId, containerGroupId); err != nil {
			fmt.Printf("Error changing ownership of keyfile: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Key written to %s\n", keyFilePath)
	},
}

func init() {
	pupCmd.AddCommand(writeKeyCmd)

	writeKeyCmd.Flags().StringP("pupId", "p", "", "ID of the pup to write the key for (required, alphanumeric only)")
	writeKeyCmd.MarkFlagRequired("pupId")

	writeKeyCmd.Flags().StringP("data-dir", "d", "", "Absolute path to the data directory (required)")
	writeKeyCmd.MarkFlagRequired("data-dir")

	writeKeyCmd.Flags().StringP("key-file", "K", "", "name of the keyfile to write to (required)")
	writeKeyCmd.MarkFlagRequired("key-file")

	writeKeyCmd.Flags().StringP("data", "k", "", "The key data to be written to the file (required)")
	writeKeyCmd.MarkFlagRequired("data")
}
