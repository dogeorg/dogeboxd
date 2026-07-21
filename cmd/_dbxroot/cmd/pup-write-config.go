package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Dogebox-WG/dogeboxd/cmd/_dbxroot/utils"
	"github.com/spf13/cobra"
)

const (
	configDirName  = ".dbx"
	configFileName = "config.env"
	configFilePerm = 0600
)

var writeConfigCmd = &cobra.Command{
	Use:   "write-config",
	Short: "Write configuration to a pup's storage",
	Long: `Write configuration environment variables to a pup's storage directory.
This command requires --pupId, --data-dir, and --config flags.
The config should be a JSON object with string key-value pairs.

Example:
  pup write-config --pupId 1234 --data-dir /absolute/path/to/data --config '{"RPC_USERNAME":"user","RPC_PASSWORD":"secret"}'`,
	Run: func(cmd *cobra.Command, args []string) {
		pupId, _ := cmd.Flags().GetString("pupId")
		dataDir, _ := cmd.Flags().GetString("data-dir")
		configJSON, _ := cmd.Flags().GetString("config")

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

		// Parse the JSON config
		var config map[string]string
		if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
			fmt.Printf("Error parsing config JSON: %v\n", err)
			os.Exit(1)
		}

		// Create the .dbx directory if it doesn't exist
		dbxDir := filepath.Join(storagePath, configDirName)
		if err := os.MkdirAll(dbxDir, storageDirPerm); err != nil {
			fmt.Printf("Error creating .dbx directory: %v\n", err)
			os.Exit(1)
		}

		// Set ownership on .dbx directory
		if err := os.Chown(dbxDir, containerUserId, containerGroupId); err != nil {
			fmt.Printf("Error changing ownership of .dbx directory: %v\n", err)
			os.Exit(1)
		}

		// Build the config.env content
		// Sort keys for deterministic output
		keys := make([]string, 0, len(config))
		for k := range config {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		var lines []string
		for _, k := range keys {
			// Escape any special characters in the value for shell/systemd
			v := config[k]
			// Values with spaces, quotes, or special chars should be quoted
			// For systemd EnvironmentFile, we use simple quoting
			if strings.ContainsAny(v, " \t\n\"'\\$`") {
				// Escape backslashes and double quotes, then wrap in double quotes
				v = strings.ReplaceAll(v, "\\", "\\\\")
				v = strings.ReplaceAll(v, "\"", "\\\"")
				v = fmt.Sprintf("\"%s\"", v)
			}
			lines = append(lines, fmt.Sprintf("%s=%s", k, v))
		}

		content := strings.Join(lines, "\n")
		if len(lines) > 0 {
			content += "\n"
		}

		// Write the config file
		configFilePath := filepath.Join(dbxDir, configFileName)
		if err := os.WriteFile(configFilePath, []byte(content), configFilePerm); err != nil {
			fmt.Printf("Error writing config file: %v\n", err)
			os.Exit(1)
		}

		// Set ownership on config file
		if err := os.Chown(configFilePath, containerUserId, containerGroupId); err != nil {
			fmt.Printf("Error changing ownership of config file: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Config written to %s\n", configFilePath)
	},
}

func init() {
	pupCmd.AddCommand(writeConfigCmd)

	writeConfigCmd.Flags().StringP("pupId", "p", "", "ID of the pup to write config for (required, alphanumeric only)")
	writeConfigCmd.MarkFlagRequired("pupId")

	writeConfigCmd.Flags().StringP("data-dir", "d", "", "Absolute path to the data directory (required)")
	writeConfigCmd.MarkFlagRequired("data-dir")

	writeConfigCmd.Flags().StringP("config", "c", "", "JSON object with config key-value pairs (required)")
	writeConfigCmd.MarkFlagRequired("config")
}
